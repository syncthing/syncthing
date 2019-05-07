/**
 * Usage: node test.js
 */

var wurl = require('./wurl.js'),
    assert = require('assert');

function strictEqual(a, b) {
  console.log('Test: ' + a + ' === ' + b);
  assert.strictEqual.apply(null, arguments);
}

// Test URLs.
var url = 'http://rob:abcd1234@www.domain.com/path/index.html?query1=test&silly=willy#test=hash&chucky=cheese',
    urlHttps = 'https://rob:abcd1234@www.domain.com/path/index.html?query1=test&silly=willy#test=hash&chucky=cheese',
    urlIp = 'https://rob:abcd1234@1.2.3.4/path/index.html?query1=test&silly=willy#test=hash&chucky=cheese';

/*strictEqual( wurl('{}', url), {
  'auth': 'rob:abcd1234',
  'domain': 'domain.com',
  'file': 'index.html',
  'fileext': 'html',
  'filename': 'index',
  'hash': 'test=hash&chucky=cheese',
  'hostname': 'www.domain.com',
  'pass': 'abcd1234',
  'path': '/path/index.html',
  'port': '80',
  'protocol': 'http',
  'query': 'query1=test&silly=willy',
  'sub': 'www',
  'tld': 'com',
  'user': 'rob'
});*/
  
strictEqual( wurl('tld', 'http://sub.www.domain.co.uk'), 'co.uk' );
strictEqual( wurl('tld', 'http://www.domain.org.uk'), 'org.uk' );
strictEqual( wurl('tld', 'http://domain.la'), 'la' );
strictEqual( wurl('tld', 'http://in'), undefined );
strictEqual( wurl('tld', 'http://.asia'), 'asia' );
strictEqual( wurl('tld', 'http://.cao.uk'), undefined );
strictEqual( wurl('tld', 'http://'), undefined );
strictEqual( wurl('tld', 'http://domain.zoo'), undefined );
strictEqual( wurl('tld', url), 'com' );

strictEqual( wurl('domain', 'http://sub.www.domain.co.uk'), 'domain.co.uk' );
strictEqual( wurl('domain', 'http://www.domain.org.uk'), 'domain.org.uk' );
strictEqual( wurl('domain', 'http://domain.la'), 'domain.la' );
strictEqual( wurl('domain', 'http://in'), undefined );
strictEqual( wurl('domain', 'http://.asia'), undefined );
strictEqual( wurl('domain', 'http://.cao.uk'), undefined );
strictEqual( wurl('domain', 'http://'), undefined );
strictEqual( wurl('domain', 'http://domain.zoo'), undefined );
strictEqual( wurl('domain', url), 'domain.com' );
strictEqual( wurl('domain', 'https://test.testshi.cn/test.html' ), 'testshi.cn' );

strictEqual( wurl('sub', 'http://sub.www.domain.co.uk'), 'sub.www' );
strictEqual( wurl('sub', 'http://www.domain.org.uk'), 'www' );
strictEqual( wurl('sub', 'http://domain.la'), undefined );
strictEqual( wurl('sub', 'http://in'), undefined );
strictEqual( wurl('sub', 'http://.asia'), undefined );
strictEqual( wurl('sub', 'http://.cao.uk'), undefined );
strictEqual( wurl('sub', 'http://'), undefined );
strictEqual( wurl('sub', 'http://domain.zoo'), undefined );
strictEqual( wurl('sub', url), 'www' );

strictEqual( wurl( 'hostname', url ), 'www.domain.com' );
strictEqual( wurl( 'hostname', urlIp ), '1.2.3.4' );

//strictEqual( wurl( '.', url ), ['www', 'domain', 'com'] );
strictEqual( wurl( '.0', url ), undefined );
strictEqual( wurl( '.1', url ), 'www' );
strictEqual( wurl( '.2', url ), 'domain' );
strictEqual( wurl( '.-1', url ), 'com' );

strictEqual( wurl( 'auth', url ), 'rob:abcd1234' );

strictEqual( wurl( 'user', url ), 'rob' );
strictEqual( wurl( 'email', 'mailto:rob@websanova.com' ), 'rob@websanova.com' );

strictEqual( wurl( 'pass', url ), 'abcd1234' );

strictEqual( wurl( 'port', url ), '80' );
strictEqual( wurl( 'port', url.toUpperCase() ), '80' );
strictEqual( wurl( 'port', 'http://example.com:80' ), '80' );
strictEqual( wurl( 'port', urlHttps ), '443' );
strictEqual( wurl( 'port', urlHttps.toUpperCase() ), '443' );
strictEqual( wurl( 'port', 'https://example.com:443' ), '443' );
strictEqual( wurl( 'port', 'http://domain.com:400?poo=a:b' ), '400' );
strictEqual( wurl( 'port', 'https://domain.com:80' ), '80' );
strictEqual( wurl( 'port', 'http://domain.com:443' ), '443' );
strictEqual( wurl( 'port', 'http://domain.com' ), '80' );
strictEqual( wurl( 'port', 'https://domain.com' ), '443' );

strictEqual( wurl( 'protocol', 'http://domain.com' ), 'http' );
strictEqual( wurl( 'protocol', 'http://domain.com:80' ), 'http' );
strictEqual( wurl( 'protocol', 'http://domain.com:443' ), 'http' );
strictEqual( wurl( 'protocol', 'domain.com' ), 'http' );
strictEqual( wurl( 'protocol', 'domain.com:80' ), 'http' );
strictEqual( wurl( 'protocol', 'domain.com:443' ), 'https' );
strictEqual( wurl( 'protocol', 'https://domain.com:443' ), 'https' );
strictEqual( wurl( 'protocol', 'https://domain.com:80' ), 'https' );
strictEqual( wurl( 'protocol', 'mailto:rob@websanova.com' ), 'mailto' );

strictEqual( wurl( 'path', url ), '/path/index.html' );
strictEqual( wurl( 'path', 'http://www.domain.com/first/second' ), '/first/second' );
strictEqual( wurl( 'path', 'http://www.domain.com/first/second/' ), '/first/second/' );
strictEqual( wurl( 'path', 'http://www.domain.com:8080/first/second' ), '/first/second' );
strictEqual( wurl( 'path', 'http://www.domain.com:8080/first/second/' ), '/first/second/' );
strictEqual( wurl( 'path', 'http://www.domain.com/first/second?test=foo' ), '/first/second' );
strictEqual( wurl( 'path', 'http://www.domain.com/first/second/?test=foo' ), '/first/second/' );
strictEqual( wurl( 'path', 'http://www.domain.com/path#anchor' ), '/path' );
strictEqual( wurl( 'path', 'http://www.domain.com/path/#anchor' ), '/path/' );
strictEqual( wurl( 'path', 'http://www.domain.com' ), '' );
strictEqual( wurl( 'path', 'http://www.domain.com/' ), '/' );
strictEqual( wurl( 'path', 'http://www.domain.com#anchor' ), '' );
strictEqual( wurl( 'path', 'http://www.domain.com/#anchor' ), '/' );
strictEqual( wurl( 'path', 'http://www.domain.com?test=foo' ), '' );
strictEqual( wurl( 'path', 'http://www.domain.com/?test=foo' ), '/' );
strictEqual( wurl( 'path', 'http://www.domain.com:80' ), '' );
strictEqual( wurl( 'path', 'http://www.domain.com:80/' ), '/' );
strictEqual( wurl( 'path', 'http://www.domain.com:80#anchor' ), '' );
strictEqual( wurl( 'path', 'http://www.domain.com:80/#anchor' ), '/' );
strictEqual( wurl( 'path', 'http://www.domain.com:80?test=foo' ), '' );
strictEqual( wurl( 'path', 'http://www.domain.com:80/?test=foo' ), '/' );

strictEqual( wurl( 'file', url ), 'index.html' );
strictEqual( wurl( 'filename', url ), 'index' );
strictEqual( wurl( 'fileext', url ), 'html' );

//strictEqual( wurl( '/', url ), ['path', 'index.html'] );
strictEqual( wurl( '0', url ), undefined );
strictEqual( wurl( '-4', url ), undefined );
strictEqual( wurl( '1', url ), 'path' );
strictEqual( wurl( 1, url ), 'path' );
strictEqual( wurl( '2', url ), 'index.html' );
strictEqual( wurl( '3', url ), undefined );
strictEqual( wurl( '-1', url ), 'index.html' );
strictEqual( wurl( '1', 'http://www.domain.com/first/second' ), 'first' );
strictEqual( wurl( '1', 'http://www.domain.com/first/second/' ), 'first' );
strictEqual( wurl( '-1', 'http://www.domain.com/first/second?test=foo' ), 'second' );
strictEqual( wurl( '-1', 'http://www.domain.com/first/second/?test=foo' ), '' );
strictEqual( wurl( '-2', 'http://www.domain.com/first/second/?test=foo' ), 'second' );

strictEqual( wurl( 'query', url ), 'query1=test&silly=willy' );
//strictEqual( wurl( '?', url ), {'query1': 'test', 'silly': 'willy'} );
strictEqual( wurl( '?silly', url ), 'willy' );
strictEqual( wurl( '?poo', url ), undefined );
strictEqual( wurl( '?poo', 'http://domain.com?poo=' ), '' );
strictEqual( wurl( '?poo', 'http://domain.com/?poo' ), '' );
strictEqual( wurl( '?poo', 'http://domain.com?poo' ), '' );
strictEqual( wurl( '?poo', 'http://domain.com?' ), undefined );
strictEqual( wurl( '?poo', 'http://domain.com' ), undefined );
strictEqual( wurl( '?poo', 'http://domain.com?poo=a+b' ), 'a b' );
strictEqual( wurl( '?poo', 'http://domain.com?poo=javascript%20decode%20uri%20%2B%20sign%20to%20space' ), 'javascript decode uri + sign to space' );
strictEqual( wurl( '?key', 'http://domain.com?key=value=va?key2=value' ), 'value=va?key2=value');
strictEqual( wurl( '?poo', 'http://domain.com:400?poo=a:b' ), 'a:b' );
strictEqual( wurl( '?poo', 'http://domain.com:400? & & &' ), undefined );

strictEqual( wurl( '?field[0]', 'http://domain.com?field[0]=zero&field[1]=one' ), 'zero' );
//strictEqual( wurl( '?field', 'http://domain.com?field[0]=zero&field[1]=one&var=test' ), ['zero', 'one'] );
//strictEqual( wurl( '?field', 'http://domain.com?field[0]=zero&field[3]=one&var=test' ), ['zero', undefined, undefined, 'one'] );
strictEqual( wurl( '?var', 'http://domain.com?field[0]=zero&field[3]=one&var=test' ), 'test' );
//strictEqual( wurl( '?', 'http://domain.com?field[0]=zero&field[1]=one&var=test' ), {'field': ['zero', 'one'], 'var': 'test'} );

strictEqual( wurl( 'hash', url ), 'test=hash&chucky=cheese' );
//strictEqual( wurl( '#', url ), {'chucky': 'cheese', 'test': 'hash'} );
strictEqual( wurl( '#chucky', url ), 'cheese' );
strictEqual( wurl( '#poo', url ), undefined );
strictEqual( wurl( '#poo', 'http://domain.com#poo=' ), '' );
strictEqual( wurl( '#poo', 'http://domain.com/#poo' ), '' );
strictEqual( wurl( '#poo', 'http://domain.com#poo' ), '' );
strictEqual( wurl( '#poo', 'http://domain.com#' ), undefined );
strictEqual( wurl( '#poo', 'http://domain.com' ), undefined );

strictEqual( wurl( '#field[0]', 'http://domain.com#field[0]=zero&field[1]=one' ), 'zero' );
//strictEqual( wurl( '#field', 'http://domain.com#field[0]=zero&field[1]=one&var=test' ), ['zero', 'one'] );
//strictEqual( wurl( '#field', 'http://domain.com#field[0]=zero&field[3]=one&var=test' ), ['zero', undefined, undefined, 'one'] );
strictEqual( wurl( '#var', 'http://domain.com#field[0]=zero&field[3]=one&var=test' ), 'test' );
//strictEqual( wurl( '#', 'http://domain.com#field[0]=zero&field[1]=one&var=test' ), {'field': ['zero', 'one'], 'var': 'test'} );
