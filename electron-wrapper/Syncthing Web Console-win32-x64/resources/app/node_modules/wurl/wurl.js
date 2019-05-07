module.exports = function (arg, url) {

    function _t() {
        return new RegExp(/(.*?)\.?([^\.]*?)\.(gl|com|net|org|biz|ws|in|me|co\.uk|co|org\.uk|ltd\.uk|plc\.uk|me\.uk|edu|mil|br\.com|cn\.com|eu\.com|hu\.com|no\.com|qc\.com|sa\.com|se\.com|se\.net|us\.com|uy\.com|ac|co\.ac|gv\.ac|or\.ac|ac\.ac|af|am|as|at|ac\.at|co\.at|gv\.at|or\.at|asn\.au|com\.au|edu\.au|org\.au|net\.au|id\.au|be|ac\.be|adm\.br|adv\.br|am\.br|arq\.br|art\.br|bio\.br|cng\.br|cnt\.br|com\.br|ecn\.br|eng\.br|esp\.br|etc\.br|eti\.br|fm\.br|fot\.br|fst\.br|g12\.br|gov\.br|ind\.br|inf\.br|jor\.br|lel\.br|med\.br|mil\.br|net\.br|nom\.br|ntr\.br|odo\.br|org\.br|ppg\.br|pro\.br|psc\.br|psi\.br|rec\.br|slg\.br|tmp\.br|tur\.br|tv\.br|vet\.br|zlg\.br|br|ab\.ca|bc\.ca|mb\.ca|nb\.ca|nf\.ca|ns\.ca|nt\.ca|on\.ca|pe\.ca|qc\.ca|sk\.ca|yk\.ca|ca|cc|ac\.cn|com\.cn|edu\.cn|gov\.cn|org\.cn|bj\.cn|sh\.cn|tj\.cn|cq\.cn|he\.cn|nm\.cn|ln\.cn|jl\.cn|hl\.cn|js\.cn|zj\.cn|ah\.cn|gd\.cn|gx\.cn|hi\.cn|sc\.cn|gz\.cn|yn\.cn|xz\.cn|sn\.cn|gs\.cn|qh\.cn|nx\.cn|xj\.cn|tw\.cn|hk\.cn|mo\.cn|cn|cx|cz|de|dk|fo|com\.ec|tm\.fr|com\.fr|asso\.fr|presse\.fr|fr|gf|gs|co\.il|net\.il|ac\.il|k12\.il|gov\.il|muni\.il|ac\.in|co\.in|org\.in|ernet\.in|gov\.in|net\.in|res\.in|is|it|ac\.jp|co\.jp|go\.jp|or\.jp|ne\.jp|ac\.kr|co\.kr|go\.kr|ne\.kr|nm\.kr|or\.kr|li|lt|lu|asso\.mc|tm\.mc|com\.mm|org\.mm|net\.mm|edu\.mm|gov\.mm|ms|nl|no|nu|pl|ro|org\.ro|store\.ro|tm\.ro|firm\.ro|www\.ro|arts\.ro|rec\.ro|info\.ro|nom\.ro|nt\.ro|se|si|com\.sg|org\.sg|net\.sg|gov\.sg|sk|st|tf|ac\.th|co\.th|go\.th|mi\.th|net\.th|or\.th|tm|to|com\.tr|edu\.tr|gov\.tr|k12\.tr|net\.tr|org\.tr|com\.tw|org\.tw|net\.tw|ac\.uk|uk\.com|uk\.net|gb\.com|gb\.net|vg|sh|kz|ch|info|ua|gov|name|pro|ie|hk|com\.hk|org\.hk|net\.hk|edu\.hk|us|tk|cd|by|ad|lv|eu\.lv|bz|es|jp|cl|ag|mobi|eu|co\.nz|org\.nz|net\.nz|maori\.nz|iwi\.nz|io|la|md|sc|sg|vc|tw|travel|my|se|tv|pt|com\.pt|edu\.pt|asia|fi|com\.ve|net\.ve|fi|org\.ve|web\.ve|info\.ve|co\.ve|tel|im|gr|ru|net\.ru|org\.ru|hr|com\.hr|ly|xyz)$/);
    }

    function _d(s) {
      return decodeURIComponent(s.replace(/\+/g, ' '));
    }

    function _i(arg, str) {
        var sptr = arg.charAt(0),
            split = str.split(sptr);

        if (sptr === arg) { return split; }

        arg = parseInt(arg.substring(1), 10);

        return split[arg < 0 ? split.length + arg : arg - 1];
    }

    function _f(arg, str) {
        var sptr = arg.charAt(0),
            split = str.split('&'),
            field = [],
            params = {},
            tmp = [],
            arg2 = arg.substring(1);

        for (var i = 0, ii = split.length; i < ii; i++) {
            field = split[i].match(/(.*?)=(.*)/);

            // TODO: regex should be able to handle this.
            if ( ! field) {
                field = [split[i], split[i], ''];
            }

            if (field[1].replace(/\s/g, '') !== '') {
                field[2] = _d(field[2] || '');

                // If we have a match just return it right away.
                if (arg2 === field[1]) { return field[2]; }

                // Check for array pattern.
                tmp = field[1].match(/(.*)\[([0-9]+)\]/);

                if (tmp) {
                    params[tmp[1]] = params[tmp[1]] || [];
                
                    params[tmp[1]][tmp[2]] = field[2];
                }
                else {
                    params[field[1]] = field[2];
                }
            }
        }

        if (sptr === arg) { return params; }

        return params[arg2];
    }

    var _l = {}, tmp, tmp2;

    if (arg === 'tld?') { return _t(); }

    url = url || window.location.toString();

    if ( ! arg) { return url; }

    arg = arg.toString();

    if (tmp = url.match(/^mailto:([^\/].+)/)) {
        _l.protocol = 'mailto';
        _l.email = tmp[1];
    }
    else {

        // Ignore Hashbangs.
        if (tmp = url.match(/(.*?)\/#\!(.*)/)) {
            url = tmp[1] + tmp[2];
        }

        // Hash.
        if (tmp = url.match(/(.*?)#(.*)/)) {
            _l.hash = tmp[2];
            url = tmp[1];
        }

        // Return hash parts.
        if (_l.hash && arg.match(/^#/)) { return _f(arg, _l.hash); }

        // Query
        if (tmp = url.match(/(.*?)\?(.*)/)) {
            _l.query = tmp[2];
            url = tmp[1];
        }

        // Return query parts.
        if (_l.query && arg.match(/^\?/)) { return _f(arg, _l.query); }

        // Protocol.
        if (tmp = url.match(/(.*?)\:?\/\/(.*)/)) {
            _l.protocol = tmp[1].toLowerCase();
            url = tmp[2];
        }

        // Path.
        if (tmp = url.match(/(.*?)(\/.*)/)) {
            _l.path = tmp[2];
            url = tmp[1];
        }

        // Clean up path.
        _l.path = (_l.path || '').replace(/^([^\/])/, '/$1');

        // Return path parts.
        if (arg.match(/^[\-0-9]+$/)) { arg = arg.replace(/^([^\/])/, '/$1'); }
        if (arg.match(/^\//)) { return _i(arg, _l.path.substring(1)); }

        // File.
        tmp = _i('/-1', _l.path.substring(1));
        
        if (tmp && (tmp = tmp.match(/(.*?)\.([^.]+)$/))) {
            _l.file = tmp[0];
            _l.filename = tmp[1];
            _l.fileext = tmp[2];
        }

        // Port.
        if (tmp = url.match(/(.*)\:([0-9]+)$/)) {
            _l.port = tmp[2];
            url = tmp[1];
        }

        // Auth.
        if (tmp = url.match(/(.*?)@(.*)/)) {
            _l.auth = tmp[1];
            url = tmp[2];
        }

        // User and pass.
        if (_l.auth) {
            tmp = _l.auth.match(/(.*)\:(.*)/);

            _l.user = tmp ? tmp[1] : _l.auth;
            _l.pass = tmp ? tmp[2] : undefined;
        }

        // Hostname.
        _l.hostname = url.toLowerCase();

        // Return hostname parts.
        if (arg.charAt(0) === '.') { return _i(arg, _l.hostname); }

        // Domain, tld and sub domain.
        if (_t()) {
            tmp = _l.hostname.match(_t());

            if (tmp) {
                _l.tld = tmp[3];
                _l.domain = tmp[2] ? tmp[2] + '.' + tmp[3] : undefined;
                _l.sub = tmp[1] || undefined;
            }
        }

        // Set port and protocol defaults if not set.
        _l.port = _l.port || (_l.protocol === 'https' ? '443' : '80');
        _l.protocol = _l.protocol || (_l.port === '443' ? 'https' : 'http');
    }

    // Return arg.
    if (arg in _l) { return _l[arg]; }

    // Return everything.
    if (arg === '{}') { return _l; }

    // Default to undefined for no match.
    return undefined;
};