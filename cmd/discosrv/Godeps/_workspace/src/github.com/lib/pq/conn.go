package pq

import (
	"bufio"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/lib/pq/oid"
)

// Common error types
var (
	ErrNotSupported              = errors.New("pq: Unsupported command")
	ErrInFailedTransaction       = errors.New("pq: Could not complete operation in a failed transaction")
	ErrSSLNotSupported           = errors.New("pq: SSL is not enabled on the server")
	ErrSSLKeyHasWorldPermissions = errors.New("pq: Private key file has group or world access. Permissions should be u=rw (0600) or less.")
	ErrCouldNotDetectUsername    = errors.New("pq: Could not detect default username. Please provide one explicitly.")
)

type drv struct{}

func (d *drv) Open(name string) (driver.Conn, error) {
	return Open(name)
}

func init() {
	sql.Register("postgres", &drv{})
}

type parameterStatus struct {
	// server version in the same format as server_version_num, or 0 if
	// unavailable
	serverVersion int

	// the current location based on the TimeZone value of the session, if
	// available
	currentLocation *time.Location
}

type transactionStatus byte

const (
	txnStatusIdle                transactionStatus = 'I'
	txnStatusIdleInTransaction   transactionStatus = 'T'
	txnStatusInFailedTransaction transactionStatus = 'E'
)

func (s transactionStatus) String() string {
	switch s {
	case txnStatusIdle:
		return "idle"
	case txnStatusIdleInTransaction:
		return "idle in transaction"
	case txnStatusInFailedTransaction:
		return "in a failed transaction"
	default:
		errorf("unknown transactionStatus %d", s)
	}

	panic("not reached")
}

type Dialer interface {
	Dial(network, address string) (net.Conn, error)
	DialTimeout(network, address string, timeout time.Duration) (net.Conn, error)
}

type defaultDialer struct{}

func (d defaultDialer) Dial(ntw, addr string) (net.Conn, error) {
	return net.Dial(ntw, addr)
}
func (d defaultDialer) DialTimeout(ntw, addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(ntw, addr, timeout)
}

type conn struct {
	c         net.Conn
	buf       *bufio.Reader
	namei     int
	scratch   [512]byte
	txnStatus transactionStatus

	parameterStatus parameterStatus

	saveMessageType   byte
	saveMessageBuffer []byte

	// If true, this connection is bad and all public-facing functions should
	// return ErrBadConn.
	bad bool
}

func (c *conn) writeBuf(b byte) *writeBuf {
	c.scratch[0] = b
	w := writeBuf(c.scratch[:5])
	return &w
}

func Open(name string) (_ driver.Conn, err error) {
	return DialOpen(defaultDialer{}, name)
}

func DialOpen(d Dialer, name string) (_ driver.Conn, err error) {
	// Handle any panics during connection initialization.  Note that we
	// specifically do *not* want to use errRecover(), as that would turn any
	// connection errors into ErrBadConns, hiding the real error message from
	// the user.
	defer errRecoverNoErrBadConn(&err)

	o := make(values)

	// A number of defaults are applied here, in this order:
	//
	// * Very low precedence defaults applied in every situation
	// * Environment variables
	// * Explicitly passed connection information
	o.Set("host", "localhost")
	o.Set("port", "5432")
	// N.B.: Extra float digits should be set to 3, but that breaks
	// Postgres 8.4 and older, where the max is 2.
	o.Set("extra_float_digits", "2")
	for k, v := range parseEnviron(os.Environ()) {
		o.Set(k, v)
	}

	if strings.HasPrefix(name, "postgres://") || strings.HasPrefix(name, "postgresql://") {
		name, err = ParseURL(name)
		if err != nil {
			return nil, err
		}
	}

	if err := parseOpts(name, o); err != nil {
		return nil, err
	}

	// Use the "fallback" application name if necessary
	if fallback := o.Get("fallback_application_name"); fallback != "" {
		if !o.Isset("application_name") {
			o.Set("application_name", fallback)
		}
	}

	// We can't work with any client_encoding other than UTF-8 currently.
	// However, we have historically allowed the user to set it to UTF-8
	// explicitly, and there's no reason to break such programs, so allow that.
	// Note that the "options" setting could also set client_encoding, but
	// parsing its value is not worth it.  Instead, we always explicitly send
	// client_encoding as a separate run-time parameter, which should override
	// anything set in options.
	if enc := o.Get("client_encoding"); enc != "" && !isUTF8(enc) {
		return nil, errors.New("client_encoding must be absent or 'UTF8'")
	}
	o.Set("client_encoding", "UTF8")
	// DateStyle needs a similar treatment.
	if datestyle := o.Get("datestyle"); datestyle != "" {
		if datestyle != "ISO, MDY" {
			panic(fmt.Sprintf("setting datestyle must be absent or %v; got %v",
				"ISO, MDY", datestyle))
		}
	} else {
		o.Set("datestyle", "ISO, MDY")
	}

	// If a user is not provided by any other means, the last
	// resort is to use the current operating system provided user
	// name.
	if o.Get("user") == "" {
		u, err := userCurrent()
		if err != nil {
			return nil, err
		} else {
			o.Set("user", u)
		}
	}

	c, err := dial(d, o)
	if err != nil {
		return nil, err
	}

	cn := &conn{c: c}
	cn.ssl(o)
	cn.buf = bufio.NewReader(cn.c)
	cn.startup(o)
	// reset the deadline, in case one was set (see dial)
	if timeout := o.Get("connect_timeout"); timeout != "" && timeout != "0" {
		err = cn.c.SetDeadline(time.Time{})
	}
	return cn, err
}

func dial(d Dialer, o values) (net.Conn, error) {
	ntw, addr := network(o)
	// SSL is not necessary or supported over UNIX domain sockets
	if ntw == "unix" {
		o["sslmode"] = "disable"
	}

	// Zero or not specified means wait indefinitely.
	if timeout := o.Get("connect_timeout"); timeout != "" && timeout != "0" {
		seconds, err := strconv.ParseInt(timeout, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("invalid value for parameter connect_timeout: %s", err)
		}
		duration := time.Duration(seconds) * time.Second
		// connect_timeout should apply to the entire connection establishment
		// procedure, so we both use a timeout for the TCP connection
		// establishment and set a deadline for doing the initial handshake.
		// The deadline is then reset after startup() is done.
		deadline := time.Now().Add(duration)
		conn, err := d.DialTimeout(ntw, addr, duration)
		if err != nil {
			return nil, err
		}
		err = conn.SetDeadline(deadline)
		return conn, err
	}
	return d.Dial(ntw, addr)
}

func network(o values) (string, string) {
	host := o.Get("host")

	if strings.HasPrefix(host, "/") {
		sockPath := path.Join(host, ".s.PGSQL."+o.Get("port"))
		return "unix", sockPath
	}

	return "tcp", host + ":" + o.Get("port")
}

type values map[string]string

func (vs values) Set(k, v string) {
	vs[k] = v
}

func (vs values) Get(k string) (v string) {
	return vs[k]
}

func (vs values) Isset(k string) bool {
	_, ok := vs[k]
	return ok
}

// scanner implements a tokenizer for libpq-style option strings.
type scanner struct {
	s []rune
	i int
}

// newScanner returns a new scanner initialized with the option string s.
func newScanner(s string) *scanner {
	return &scanner{[]rune(s), 0}
}

// Next returns the next rune.
// It returns 0, false if the end of the text has been reached.
func (s *scanner) Next() (rune, bool) {
	if s.i >= len(s.s) {
		return 0, false
	}
	r := s.s[s.i]
	s.i++
	return r, true
}

// SkipSpaces returns the next non-whitespace rune.
// It returns 0, false if the end of the text has been reached.
func (s *scanner) SkipSpaces() (rune, bool) {
	r, ok := s.Next()
	for unicode.IsSpace(r) && ok {
		r, ok = s.Next()
	}
	return r, ok
}

// parseOpts parses the options from name and adds them to the values.
//
// The parsing code is based on conninfo_parse from libpq's fe-connect.c
func parseOpts(name string, o values) error {
	s := newScanner(name)

	for {
		var (
			keyRunes, valRunes []rune
			r                  rune
			ok                 bool
		)

		if r, ok = s.SkipSpaces(); !ok {
			break
		}

		// Scan the key
		for !unicode.IsSpace(r) && r != '=' {
			keyRunes = append(keyRunes, r)
			if r, ok = s.Next(); !ok {
				break
			}
		}

		// Skip any whitespace if we're not at the = yet
		if r != '=' {
			r, ok = s.SkipSpaces()
		}

		// The current character should be =
		if r != '=' || !ok {
			return fmt.Errorf(`missing "=" after %q in connection info string"`, string(keyRunes))
		}

		// Skip any whitespace after the =
		if r, ok = s.SkipSpaces(); !ok {
			// If we reach the end here, the last value is just an empty string as per libpq.
			o.Set(string(keyRunes), "")
			break
		}

		if r != '\'' {
			for !unicode.IsSpace(r) {
				if r == '\\' {
					if r, ok = s.Next(); !ok {
						return fmt.Errorf(`missing character after backslash`)
					}
				}
				valRunes = append(valRunes, r)

				if r, ok = s.Next(); !ok {
					break
				}
			}
		} else {
		quote:
			for {
				if r, ok = s.Next(); !ok {
					return fmt.Errorf(`unterminated quoted string literal in connection string`)
				}
				switch r {
				case '\'':
					break quote
				case '\\':
					r, _ = s.Next()
					fallthrough
				default:
					valRunes = append(valRunes, r)
				}
			}
		}

		o.Set(string(keyRunes), string(valRunes))
	}

	return nil
}

func (cn *conn) isInTransaction() bool {
	return cn.txnStatus == txnStatusIdleInTransaction ||
		cn.txnStatus == txnStatusInFailedTransaction
}

func (cn *conn) checkIsInTransaction(intxn bool) {
	if cn.isInTransaction() != intxn {
		cn.bad = true
		errorf("unexpected transaction status %v", cn.txnStatus)
	}
}

func (cn *conn) Begin() (_ driver.Tx, err error) {
	if cn.bad {
		return nil, driver.ErrBadConn
	}
	defer cn.errRecover(&err)

	cn.checkIsInTransaction(false)
	_, commandTag, err := cn.simpleExec("BEGIN")
	if err != nil {
		return nil, err
	}
	if commandTag != "BEGIN" {
		cn.bad = true
		return nil, fmt.Errorf("unexpected command tag %s", commandTag)
	}
	if cn.txnStatus != txnStatusIdleInTransaction {
		cn.bad = true
		return nil, fmt.Errorf("unexpected transaction status %v", cn.txnStatus)
	}
	return cn, nil
}

func (cn *conn) Commit() (err error) {
	if cn.bad {
		return driver.ErrBadConn
	}
	defer cn.errRecover(&err)

	cn.checkIsInTransaction(true)
	// We don't want the client to think that everything is okay if it tries
	// to commit a failed transaction.  However, no matter what we return,
	// database/sql will release this connection back into the free connection
	// pool so we have to abort the current transaction here.  Note that you
	// would get the same behaviour if you issued a COMMIT in a failed
	// transaction, so it's also the least surprising thing to do here.
	if cn.txnStatus == txnStatusInFailedTransaction {
		if err := cn.Rollback(); err != nil {
			return err
		}
		return ErrInFailedTransaction
	}

	_, commandTag, err := cn.simpleExec("COMMIT")
	if err != nil {
		if cn.isInTransaction() {
			cn.bad = true
		}
		return err
	}
	if commandTag != "COMMIT" {
		cn.bad = true
		return fmt.Errorf("unexpected command tag %s", commandTag)
	}
	cn.checkIsInTransaction(false)
	return nil
}

func (cn *conn) Rollback() (err error) {
	if cn.bad {
		return driver.ErrBadConn
	}
	defer cn.errRecover(&err)

	cn.checkIsInTransaction(true)
	_, commandTag, err := cn.simpleExec("ROLLBACK")
	if err != nil {
		if cn.isInTransaction() {
			cn.bad = true
		}
		return err
	}
	if commandTag != "ROLLBACK" {
		return fmt.Errorf("unexpected command tag %s", commandTag)
	}
	cn.checkIsInTransaction(false)
	return nil
}

func (cn *conn) gname() string {
	cn.namei++
	return strconv.FormatInt(int64(cn.namei), 10)
}

func (cn *conn) simpleExec(q string) (res driver.Result, commandTag string, err error) {
	b := cn.writeBuf('Q')
	b.string(q)
	cn.send(b)

	for {
		t, r := cn.recv1()
		switch t {
		case 'C':
			res, commandTag = cn.parseComplete(r.string())
		case 'Z':
			cn.processReadyForQuery(r)
			// done
			return
		case 'E':
			err = parseError(r)
		case 'T', 'D', 'I':
			// ignore any results
		default:
			cn.bad = true
			errorf("unknown response for simple query: %q", t)
		}
	}
}

func (cn *conn) simpleQuery(q string) (res driver.Rows, err error) {
	defer cn.errRecover(&err)

	st := &stmt{cn: cn, name: ""}

	b := cn.writeBuf('Q')
	b.string(q)
	cn.send(b)

	for {
		t, r := cn.recv1()
		switch t {
		case 'C', 'I':
			// We allow queries which don't return any results through Query as
			// well as Exec.  We still have to give database/sql a rows object
			// the user can close, though, to avoid connections from being
			// leaked.  A "rows" with done=true works fine for that purpose.
			if err != nil {
				cn.bad = true
				errorf("unexpected message %q in simple query execution", t)
			}
			res = &rows{st: st, done: true}
		case 'Z':
			cn.processReadyForQuery(r)
			// done
			return
		case 'E':
			res = nil
			err = parseError(r)
		case 'D':
			if res == nil {
				cn.bad = true
				errorf("unexpected DataRow in simple query execution")
			}
			// the query didn't fail; kick off to Next
			cn.saveMessage(t, r)
			return
		case 'T':
			// res might be non-nil here if we received a previous
			// CommandComplete, but that's fine; just overwrite it
			res = &rows{st: st}
			st.cols, st.rowTyps = parseMeta(r)

			// To work around a bug in QueryRow in Go 1.2 and earlier, wait
			// until the first DataRow has been received.
		default:
			cn.bad = true
			errorf("unknown response for simple query: %q", t)
		}
	}
}

func (cn *conn) prepareTo(q, stmtName string) (_ *stmt, err error) {
	st := &stmt{cn: cn, name: stmtName}

	b := cn.writeBuf('P')
	b.string(st.name)
	b.string(q)
	b.int16(0)
	cn.send(b)

	b = cn.writeBuf('D')
	b.byte('S')
	b.string(st.name)
	cn.send(b)

	cn.send(cn.writeBuf('S'))

	for {
		t, r := cn.recv1()
		switch t {
		case '1':
		case 't':
			nparams := r.int16()
			st.paramTyps = make([]oid.Oid, nparams)

			for i := range st.paramTyps {
				st.paramTyps[i] = r.oid()
			}
		case 'T':
			st.cols, st.rowTyps = parseMeta(r)
		case 'n':
			// no data
		case 'Z':
			cn.processReadyForQuery(r)
			return st, err
		case 'E':
			err = parseError(r)
		default:
			cn.bad = true
			errorf("unexpected describe rows response: %q", t)
		}
	}
}

func (cn *conn) Prepare(q string) (_ driver.Stmt, err error) {
	if cn.bad {
		return nil, driver.ErrBadConn
	}
	defer cn.errRecover(&err)

	if len(q) >= 4 && strings.EqualFold(q[:4], "COPY") {
		return cn.prepareCopyIn(q)
	}
	return cn.prepareTo(q, cn.gname())
}

func (cn *conn) Close() (err error) {
	if cn.bad {
		return driver.ErrBadConn
	}
	defer cn.errRecover(&err)

	// Don't go through send(); ListenerConn relies on us not scribbling on the
	// scratch buffer of this connection.
	err = cn.sendSimpleMessage('X')
	if err != nil {
		return err
	}

	return cn.c.Close()
}

// Implement the "Queryer" interface
func (cn *conn) Query(query string, args []driver.Value) (_ driver.Rows, err error) {
	if cn.bad {
		return nil, driver.ErrBadConn
	}
	defer cn.errRecover(&err)

	// Check to see if we can use the "simpleQuery" interface, which is
	// *much* faster than going through prepare/exec
	if len(args) == 0 {
		return cn.simpleQuery(query)
	}

	st, err := cn.prepareTo(query, "")
	if err != nil {
		panic(err)
	}

	st.exec(args)
	return &rows{st: st}, nil
}

// Implement the optional "Execer" interface for one-shot queries
func (cn *conn) Exec(query string, args []driver.Value) (_ driver.Result, err error) {
	if cn.bad {
		return nil, driver.ErrBadConn
	}
	defer cn.errRecover(&err)

	// Check to see if we can use the "simpleExec" interface, which is
	// *much* faster than going through prepare/exec
	if len(args) == 0 {
		// ignore commandTag, our caller doesn't care
		r, _, err := cn.simpleExec(query)
		return r, err
	}

	// Use the unnamed statement to defer planning until bind
	// time, or else value-based selectivity estimates cannot be
	// used.
	st, err := cn.prepareTo(query, "")
	if err != nil {
		panic(err)
	}

	r, err := st.Exec(args)
	if err != nil {
		panic(err)
	}

	return r, err
}

// Assumes len(*m) is > 5
func (cn *conn) send(m *writeBuf) {
	b := (*m)[1:]
	binary.BigEndian.PutUint32(b, uint32(len(b)))

	if (*m)[0] == 0 {
		*m = b
	}

	_, err := cn.c.Write(*m)
	if err != nil {
		panic(err)
	}
}

// Send a message of type typ to the server on the other end of cn.  The
// message should have no payload.  This method does not use the scratch
// buffer.
func (cn *conn) sendSimpleMessage(typ byte) (err error) {
	_, err = cn.c.Write([]byte{typ, '\x00', '\x00', '\x00', '\x04'})
	return err
}

// saveMessage memorizes a message and its buffer in the conn struct.
// recvMessage will then return these values on the next call to it.  This
// method is useful in cases where you have to see what the next message is
// going to be (e.g. to see whether it's an error or not) but you can't handle
// the message yourself.
func (cn *conn) saveMessage(typ byte, buf *readBuf) {
	if cn.saveMessageType != 0 {
		cn.bad = true
		errorf("unexpected saveMessageType %d", cn.saveMessageType)
	}
	cn.saveMessageType = typ
	cn.saveMessageBuffer = *buf
}

// recvMessage receives any message from the backend, or returns an error if
// a problem occurred while reading the message.
func (cn *conn) recvMessage(r *readBuf) (byte, error) {
	// workaround for a QueryRow bug, see exec
	if cn.saveMessageType != 0 {
		t := cn.saveMessageType
		*r = cn.saveMessageBuffer
		cn.saveMessageType = 0
		cn.saveMessageBuffer = nil
		return t, nil
	}

	x := cn.scratch[:5]
	_, err := io.ReadFull(cn.buf, x)
	if err != nil {
		return 0, err
	}

	// read the type and length of the message that follows
	t := x[0]
	n := int(binary.BigEndian.Uint32(x[1:])) - 4
	var y []byte
	if n <= len(cn.scratch) {
		y = cn.scratch[:n]
	} else {
		y = make([]byte, n)
	}
	_, err = io.ReadFull(cn.buf, y)
	if err != nil {
		return 0, err
	}
	*r = y
	return t, nil
}

// recv receives a message from the backend, but if an error happened while
// reading the message or the received message was an ErrorResponse, it panics.
// NoticeResponses are ignored.  This function should generally be used only
// during the startup sequence.
func (cn *conn) recv() (t byte, r *readBuf) {
	for {
		var err error
		r = &readBuf{}
		t, err = cn.recvMessage(r)
		if err != nil {
			panic(err)
		}

		switch t {
		case 'E':
			panic(parseError(r))
		case 'N':
			// ignore
		default:
			return
		}
	}
}

// recv1Buf is exactly equivalent to recv1, except it uses a buffer supplied by
// the caller to avoid an allocation.
func (cn *conn) recv1Buf(r *readBuf) byte {
	for {
		t, err := cn.recvMessage(r)
		if err != nil {
			panic(err)
		}

		switch t {
		case 'A', 'N':
			// ignore
		case 'S':
			cn.processParameterStatus(r)
		default:
			return t
		}
	}
}

// recv1 receives a message from the backend, panicking if an error occurs
// while attempting to read it.  All asynchronous messages are ignored, with
// the exception of ErrorResponse.
func (cn *conn) recv1() (t byte, r *readBuf) {
	r = &readBuf{}
	t = cn.recv1Buf(r)
	return t, r
}

func (cn *conn) ssl(o values) {
	verifyCaOnly := false
	tlsConf := tls.Config{}
	switch mode := o.Get("sslmode"); mode {
	case "require", "":
		tlsConf.InsecureSkipVerify = true
	case "verify-ca":
		// We must skip TLS's own verification since it requires full
		// verification since Go 1.3.
		tlsConf.InsecureSkipVerify = true
		verifyCaOnly = true
	case "verify-full":
		tlsConf.ServerName = o.Get("host")
	case "disable":
		return
	default:
		errorf(`unsupported sslmode %q; only "require" (default), "verify-full", and "disable" supported`, mode)
	}

	cn.setupSSLClientCertificates(&tlsConf, o)
	cn.setupSSLCA(&tlsConf, o)

	w := cn.writeBuf(0)
	w.int32(80877103)
	cn.send(w)

	b := cn.scratch[:1]
	_, err := io.ReadFull(cn.c, b)
	if err != nil {
		panic(err)
	}

	if b[0] != 'S' {
		panic(ErrSSLNotSupported)
	}

	client := tls.Client(cn.c, &tlsConf)
	if verifyCaOnly {
		cn.verifyCA(client, &tlsConf)
	}
	cn.c = client
}

// verifyCA carries out a TLS handshake to the server and verifies the
// presented certificate against the effective CA, i.e. the one specified in
// sslrootcert or the system CA if sslrootcert was not specified.
func (cn *conn) verifyCA(client *tls.Conn, tlsConf *tls.Config) {
	err := client.Handshake()
	if err != nil {
		panic(err)
	}
	certs := client.ConnectionState().PeerCertificates
	opts := x509.VerifyOptions{
		DNSName:       client.ConnectionState().ServerName,
		Intermediates: x509.NewCertPool(),
		Roots:         tlsConf.RootCAs,
	}
	for i, cert := range certs {
		if i == 0 {
			continue
		}
		opts.Intermediates.AddCert(cert)
	}
	_, err = certs[0].Verify(opts)
	if err != nil {
		panic(err)
	}
}

// This function sets up SSL client certificates based on either the "sslkey"
// and "sslcert" settings (possibly set via the environment variables PGSSLKEY
// and PGSSLCERT, respectively), or if they aren't set, from the .postgresql
// directory in the user's home directory.  If the file paths are set
// explicitly, the files must exist.  The key file must also not be
// world-readable, or this function will panic with
// ErrSSLKeyHasWorldPermissions.
func (cn *conn) setupSSLClientCertificates(tlsConf *tls.Config, o values) {
	var missingOk bool

	sslkey := o.Get("sslkey")
	sslcert := o.Get("sslcert")
	if sslkey != "" && sslcert != "" {
		// If the user has set an sslkey and sslcert, they *must* exist.
		missingOk = false
	} else {
		// Automatically load certificates from ~/.postgresql.
		user, err := user.Current()
		if err != nil {
			// user.Current() might fail when cross-compiling.  We have to
			// ignore the error and continue without client certificates, since
			// we wouldn't know where to load them from.
			return
		}

		sslkey = filepath.Join(user.HomeDir, ".postgresql", "postgresql.key")
		sslcert = filepath.Join(user.HomeDir, ".postgresql", "postgresql.crt")
		missingOk = true
	}

	// Check that both files exist, and report the error or stop, depending on
	// which behaviour we want.  Note that we don't do any more extensive
	// checks than this (such as checking that the paths aren't directories);
	// LoadX509KeyPair() will take care of the rest.
	keyfinfo, err := os.Stat(sslkey)
	if err != nil && missingOk {
		return
	} else if err != nil {
		panic(err)
	}
	_, err = os.Stat(sslcert)
	if err != nil && missingOk {
		return
	} else if err != nil {
		panic(err)
	}

	// If we got this far, the key file must also have the correct permissions
	kmode := keyfinfo.Mode()
	if kmode != kmode&0600 {
		panic(ErrSSLKeyHasWorldPermissions)
	}

	cert, err := tls.LoadX509KeyPair(sslcert, sslkey)
	if err != nil {
		panic(err)
	}
	tlsConf.Certificates = []tls.Certificate{cert}
}

// Sets up RootCAs in the TLS configuration if sslrootcert is set.
func (cn *conn) setupSSLCA(tlsConf *tls.Config, o values) {
	if sslrootcert := o.Get("sslrootcert"); sslrootcert != "" {
		tlsConf.RootCAs = x509.NewCertPool()

		cert, err := ioutil.ReadFile(sslrootcert)
		if err != nil {
			panic(err)
		}

		ok := tlsConf.RootCAs.AppendCertsFromPEM(cert)
		if !ok {
			errorf("couldn't parse pem in sslrootcert")
		}
	}
}

// isDriverSetting returns true iff a setting is purely for configuring the
// driver's options and should not be sent to the server in the connection
// startup packet.
func isDriverSetting(key string) bool {
	switch key {
	case "host", "port":
		return true
	case "password":
		return true
	case "sslmode", "sslcert", "sslkey", "sslrootcert":
		return true
	case "fallback_application_name":
		return true
	case "connect_timeout":
		return true

	default:
		return false
	}
}

func (cn *conn) startup(o values) {
	w := cn.writeBuf(0)
	w.int32(196608)
	// Send the backend the name of the database we want to connect to, and the
	// user we want to connect as.  Additionally, we send over any run-time
	// parameters potentially included in the connection string.  If the server
	// doesn't recognize any of them, it will reply with an error.
	for k, v := range o {
		if isDriverSetting(k) {
			// skip options which can't be run-time parameters
			continue
		}
		// The protocol requires us to supply the database name as "database"
		// instead of "dbname".
		if k == "dbname" {
			k = "database"
		}
		w.string(k)
		w.string(v)
	}
	w.string("")
	cn.send(w)

	for {
		t, r := cn.recv()
		switch t {
		case 'K':
		case 'S':
			cn.processParameterStatus(r)
		case 'R':
			cn.auth(r, o)
		case 'Z':
			cn.processReadyForQuery(r)
			return
		default:
			errorf("unknown response for startup: %q", t)
		}
	}
}

func (cn *conn) auth(r *readBuf, o values) {
	switch code := r.int32(); code {
	case 0:
		// OK
	case 3:
		w := cn.writeBuf('p')
		w.string(o.Get("password"))
		cn.send(w)

		t, r := cn.recv()
		if t != 'R' {
			errorf("unexpected password response: %q", t)
		}

		if r.int32() != 0 {
			errorf("unexpected authentication response: %q", t)
		}
	case 5:
		s := string(r.next(4))
		w := cn.writeBuf('p')
		w.string("md5" + md5s(md5s(o.Get("password")+o.Get("user"))+s))
		cn.send(w)

		t, r := cn.recv()
		if t != 'R' {
			errorf("unexpected password response: %q", t)
		}

		if r.int32() != 0 {
			errorf("unexpected authentication response: %q", t)
		}
	default:
		errorf("unknown authentication response: %d", code)
	}
}

type stmt struct {
	cn        *conn
	name      string
	cols      []string
	rowTyps   []oid.Oid
	paramTyps []oid.Oid
	closed    bool
}

func (st *stmt) Close() (err error) {
	if st.closed {
		return nil
	}
	if st.cn.bad {
		return driver.ErrBadConn
	}
	defer st.cn.errRecover(&err)

	w := st.cn.writeBuf('C')
	w.byte('S')
	w.string(st.name)
	st.cn.send(w)

	st.cn.send(st.cn.writeBuf('S'))

	t, _ := st.cn.recv1()
	if t != '3' {
		st.cn.bad = true
		errorf("unexpected close response: %q", t)
	}
	st.closed = true

	t, r := st.cn.recv1()
	if t != 'Z' {
		st.cn.bad = true
		errorf("expected ready for query, but got: %q", t)
	}
	st.cn.processReadyForQuery(r)

	return nil
}

func (st *stmt) Query(v []driver.Value) (r driver.Rows, err error) {
	if st.cn.bad {
		return nil, driver.ErrBadConn
	}
	defer st.cn.errRecover(&err)

	st.exec(v)
	return &rows{st: st}, nil
}

func (st *stmt) Exec(v []driver.Value) (res driver.Result, err error) {
	if st.cn.bad {
		return nil, driver.ErrBadConn
	}
	defer st.cn.errRecover(&err)

	st.exec(v)

	for {
		t, r := st.cn.recv1()
		switch t {
		case 'E':
			err = parseError(r)
		case 'C':
			res, _ = st.cn.parseComplete(r.string())
		case 'Z':
			st.cn.processReadyForQuery(r)
			// done
			return
		case 'T', 'D', 'I':
			// ignore any results
		default:
			st.cn.bad = true
			errorf("unknown exec response: %q", t)
		}
	}
}

func (st *stmt) exec(v []driver.Value) {
	if len(v) >= 65536 {
		errorf("got %d parameters but PostgreSQL only supports 65535 parameters", len(v))
	}
	if len(v) != len(st.paramTyps) {
		errorf("got %d parameters but the statement requires %d", len(v), len(st.paramTyps))
	}

	w := st.cn.writeBuf('B')
	w.string("")
	w.string(st.name)
	w.int16(0)
	w.int16(len(v))
	for i, x := range v {
		if x == nil {
			w.int32(-1)
		} else {
			b := encode(&st.cn.parameterStatus, x, st.paramTyps[i])
			w.int32(len(b))
			w.bytes(b)
		}
	}
	w.int16(0)
	st.cn.send(w)

	w = st.cn.writeBuf('E')
	w.string("")
	w.int32(0)
	st.cn.send(w)

	st.cn.send(st.cn.writeBuf('S'))

	var err error
	for {
		t, r := st.cn.recv1()
		switch t {
		case 'E':
			err = parseError(r)
		case '2':
			if err != nil {
				panic(err)
			}
			goto workaround
		case 'Z':
			st.cn.processReadyForQuery(r)
			if err != nil {
				panic(err)
			}
			return
		default:
			st.cn.bad = true
			errorf("unexpected bind response: %q", t)
		}
	}

	// Work around a bug in sql.DB.QueryRow: in Go 1.2 and earlier it ignores
	// any errors from rows.Next, which masks errors that happened during the
	// execution of the query.  To avoid the problem in common cases, we wait
	// here for one more message from the database.  If it's not an error the
	// query will likely succeed (or perhaps has already, if it's a
	// CommandComplete), so we push the message into the conn struct; recv1
	// will return it as the next message for rows.Next or rows.Close.
	// However, if it's an error, we wait until ReadyForQuery and then return
	// the error to our caller.
workaround:
	for {
		t, r := st.cn.recv1()
		switch t {
		case 'E':
			err = parseError(r)
		case 'C', 'D', 'I':
			// the query didn't fail, but we can't process this message
			st.cn.saveMessage(t, r)
			return
		case 'Z':
			if err == nil {
				st.cn.bad = true
				errorf("unexpected ReadyForQuery during extended query execution")
			}
			st.cn.processReadyForQuery(r)
			panic(err)
		default:
			st.cn.bad = true
			errorf("unexpected message during query execution: %q", t)
		}
	}
}

func (st *stmt) NumInput() int {
	return len(st.paramTyps)
}

// parseComplete parses the "command tag" from a CommandComplete message, and
// returns the number of rows affected (if applicable) and a string
// identifying only the command that was executed, e.g. "ALTER TABLE".  If the
// command tag could not be parsed, parseComplete panics.
func (cn *conn) parseComplete(commandTag string) (driver.Result, string) {
	commandsWithAffectedRows := []string{
		"SELECT ",
		// INSERT is handled below
		"UPDATE ",
		"DELETE ",
		"FETCH ",
		"MOVE ",
		"COPY ",
	}

	var affectedRows *string
	for _, tag := range commandsWithAffectedRows {
		if strings.HasPrefix(commandTag, tag) {
			t := commandTag[len(tag):]
			affectedRows = &t
			commandTag = tag[:len(tag)-1]
			break
		}
	}
	// INSERT also includes the oid of the inserted row in its command tag.
	// Oids in user tables are deprecated, and the oid is only returned when
	// exactly one row is inserted, so it's unlikely to be of value to any
	// real-world application and we can ignore it.
	if affectedRows == nil && strings.HasPrefix(commandTag, "INSERT ") {
		parts := strings.Split(commandTag, " ")
		if len(parts) != 3 {
			cn.bad = true
			errorf("unexpected INSERT command tag %s", commandTag)
		}
		affectedRows = &parts[len(parts)-1]
		commandTag = "INSERT"
	}
	// There should be no affected rows attached to the tag, just return it
	if affectedRows == nil {
		return driver.RowsAffected(0), commandTag
	}
	n, err := strconv.ParseInt(*affectedRows, 10, 64)
	if err != nil {
		cn.bad = true
		errorf("could not parse commandTag: %s", err)
	}
	return driver.RowsAffected(n), commandTag
}

type rows struct {
	st   *stmt
	done bool
	rb   readBuf
}

func (rs *rows) Close() error {
	// no need to look at cn.bad as Next() will
	for {
		err := rs.Next(nil)
		switch err {
		case nil:
		case io.EOF:
			return nil
		default:
			return err
		}
	}
}

func (rs *rows) Columns() []string {
	return rs.st.cols
}

func (rs *rows) Next(dest []driver.Value) (err error) {
	if rs.done {
		return io.EOF
	}

	conn := rs.st.cn
	if conn.bad {
		return driver.ErrBadConn
	}
	defer conn.errRecover(&err)

	for {
		t := conn.recv1Buf(&rs.rb)
		switch t {
		case 'E':
			err = parseError(&rs.rb)
		case 'C', 'I':
			continue
		case 'Z':
			conn.processReadyForQuery(&rs.rb)
			rs.done = true
			if err != nil {
				return err
			}
			return io.EOF
		case 'D':
			n := rs.rb.int16()
			if n < len(dest) {
				dest = dest[:n]
			}
			for i := range dest {
				l := rs.rb.int32()
				if l == -1 {
					dest[i] = nil
					continue
				}
				dest[i] = decode(&conn.parameterStatus, rs.rb.next(l), rs.st.rowTyps[i])
			}
			return
		default:
			errorf("unexpected message after execute: %q", t)
		}
	}
}

// QuoteIdentifier quotes an "identifier" (e.g. a table or a column name) to be
// used as part of an SQL statement.  For example:
//
//    tblname := "my_table"
//    data := "my_data"
//    err = db.Exec(fmt.Sprintf("INSERT INTO %s VALUES ($1)", pq.QuoteIdentifier(tblname)), data)
//
// Any double quotes in name will be escaped.  The quoted identifier will be
// case sensitive when used in a query.  If the input string contains a zero
// byte, the result will be truncated immediately before it.
func QuoteIdentifier(name string) string {
	end := strings.IndexRune(name, 0)
	if end > -1 {
		name = name[:end]
	}
	return `"` + strings.Replace(name, `"`, `""`, -1) + `"`
}

func md5s(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (c *conn) processParameterStatus(r *readBuf) {
	var err error

	param := r.string()
	switch param {
	case "server_version":
		var major1 int
		var major2 int
		var minor int
		_, err = fmt.Sscanf(r.string(), "%d.%d.%d", &major1, &major2, &minor)
		if err == nil {
			c.parameterStatus.serverVersion = major1*10000 + major2*100 + minor
		}

	case "TimeZone":
		c.parameterStatus.currentLocation, err = time.LoadLocation(r.string())
		if err != nil {
			c.parameterStatus.currentLocation = nil
		}

	default:
		// ignore
	}
}

func (c *conn) processReadyForQuery(r *readBuf) {
	c.txnStatus = transactionStatus(r.byte())
}

func parseMeta(r *readBuf) (cols []string, rowTyps []oid.Oid) {
	n := r.int16()
	cols = make([]string, n)
	rowTyps = make([]oid.Oid, n)
	for i := range cols {
		cols[i] = r.string()
		r.next(6)
		rowTyps[i] = r.oid()
		r.next(8)
	}
	return
}

// parseEnviron tries to mimic some of libpq's environment handling
//
// To ease testing, it does not directly reference os.Environ, but is
// designed to accept its output.
//
// Environment-set connection information is intended to have a higher
// precedence than a library default but lower than any explicitly
// passed information (such as in the URL or connection string).
func parseEnviron(env []string) (out map[string]string) {
	out = make(map[string]string)

	for _, v := range env {
		parts := strings.SplitN(v, "=", 2)

		accrue := func(keyname string) {
			out[keyname] = parts[1]
		}
		unsupported := func() {
			panic(fmt.Sprintf("setting %v not supported", parts[0]))
		}

		// The order of these is the same as is seen in the
		// PostgreSQL 9.1 manual. Unsupported but well-defined
		// keys cause a panic; these should be unset prior to
		// execution. Options which pq expects to be set to a
		// certain value are allowed, but must be set to that
		// value if present (they can, of course, be absent).
		switch parts[0] {
		case "PGHOST":
			accrue("host")
		case "PGHOSTADDR":
			unsupported()
		case "PGPORT":
			accrue("port")
		case "PGDATABASE":
			accrue("dbname")
		case "PGUSER":
			accrue("user")
		case "PGPASSWORD":
			accrue("password")
		case "PGPASSFILE", "PGSERVICE", "PGSERVICEFILE", "PGREALM":
			unsupported()
		case "PGOPTIONS":
			accrue("options")
		case "PGAPPNAME":
			accrue("application_name")
		case "PGSSLMODE":
			accrue("sslmode")
		case "PGSSLCERT":
			accrue("sslcert")
		case "PGSSLKEY":
			accrue("sslkey")
		case "PGSSLROOTCERT":
			accrue("sslrootcert")
		case "PGREQUIRESSL", "PGSSLCRL":
			unsupported()
		case "PGREQUIREPEER":
			unsupported()
		case "PGKRBSRVNAME", "PGGSSLIB":
			unsupported()
		case "PGCONNECT_TIMEOUT":
			accrue("connect_timeout")
		case "PGCLIENTENCODING":
			accrue("client_encoding")
		case "PGDATESTYLE":
			accrue("datestyle")
		case "PGTZ":
			accrue("timezone")
		case "PGGEQO":
			accrue("geqo")
		case "PGSYSCONFDIR", "PGLOCALEDIR":
			unsupported()
		}
	}

	return out
}

// isUTF8 returns whether name is a fuzzy variation of the string "UTF-8".
func isUTF8(name string) bool {
	// Recognize all sorts of silly things as "UTF-8", like Postgres does
	s := strings.Map(alnumLowerASCII, name)
	return s == "utf8" || s == "unicode"
}

func alnumLowerASCII(ch rune) rune {
	if 'A' <= ch && ch <= 'Z' {
		return ch + ('a' - 'A')
	}
	if 'a' <= ch && ch <= 'z' || '0' <= ch && ch <= '9' {
		return ch
	}
	return -1 // discard
}
