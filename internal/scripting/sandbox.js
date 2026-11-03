// Sets up the Postman-style script API (pm, console, require, ...) on the
// global object. Runs before every script; scripting.go provides __native and
// __inputJSON, and reads the result back through __finish().
(function (global) {
  'use strict';

  var native = global.__native;
  var input = JSON.parse(global.__inputJSON);
  delete global.__native;
  delete global.__inputJSON;

  var req = input.request;
  var locals = input.locals || {};
  var tests = [];

  function str(v) {
    if (v === undefined || v === null) return '';
    if (typeof v === 'object') return JSON.stringify(v);
    return String(v);
  }

  function inspect(v) {
    if (typeof v === 'string') return "'" + (v.length > 200 ? v.slice(0, 200) + '…' : v) + "'";
    if (v === undefined) return 'undefined';
    if (typeof v === 'function') return '[Function' + (v.name ? ' ' + v.name : '') + ']';
    if (typeof v === 'number' || typeof v === 'boolean' || v === null) return String(v);
    if (v instanceof RegExp || v instanceof Date) return String(v);
    try {
      var s = JSON.stringify(v);
      return s.length > 200 ? s.slice(0, 200) + '…' : s;
    } catch (e) {
      return String(v);
    }
  }

  // ---- console ----

  function format(args) {
    return Array.prototype.map
      .call(args, function (a) {
        if (typeof a === 'string') return a;
        if (a instanceof Error) return a.name + ': ' + a.message;
        if (a === undefined) return 'undefined';
        try {
          return typeof a === 'object' && a !== null ? JSON.stringify(a, null, 2) : String(a);
        } catch (e) {
          return String(a);
        }
      })
      .join(' ');
  }
  var consoleObj = {};
  ['log', 'info', 'warn', 'error', 'debug'].forEach(function (level) {
    consoleObj[level] = function () {
      native.log(level === 'debug' ? 'log' : level, format(arguments));
    };
  });

  // ---- key/value lists (headers, query parameters) ----

  function toRow(item, value) {
    if (typeof item === 'string') {
      if (value !== undefined) return { key: item, value: str(value), enabled: true };
      var i = item.indexOf(':');
      return i < 0
        ? { key: item.trim(), value: '', enabled: true }
        : { key: item.slice(0, i).trim(), value: item.slice(i + 1).trim(), enabled: true };
    }
    if (!item || item.key === undefined) throw new TypeError('expected {key, value}');
    return { key: String(item.key), value: str(item.value), enabled: item.disabled !== true };
  }

  function PropertyList(rows, caseInsensitive) {
    Object.defineProperty(this, '_rows', { value: rows });
    Object.defineProperty(this, '_ci', { value: !!caseInsensitive });
  }
  PropertyList.prototype._is = function (row, key) {
    return this._ci ? row.key.toLowerCase() === String(key).toLowerCase() : row.key === String(key);
  };
  PropertyList.prototype._active = function () {
    return this._rows.filter(function (r) {
      return r.enabled !== false && r.key;
    });
  };
  PropertyList.prototype.get = function (key) {
    var self = this;
    var hit = this._active().filter(function (r) {
      return self._is(r, key);
    })[0];
    return hit ? hit.value : undefined;
  };
  PropertyList.prototype.has = function (key, value) {
    var self = this;
    return this._active().some(function (r) {
      return self._is(r, key) && (value === undefined || r.value === str(value));
    });
  };
  PropertyList.prototype.add = function (item, value) {
    this._rows.push(toRow(item, value));
  };
  PropertyList.prototype.upsert = function (item, value) {
    var row = toRow(item, value);
    for (var i = 0; i < this._rows.length; i++) {
      if (this._is(this._rows[i], row.key)) {
        this._rows[i].value = row.value;
        this._rows[i].enabled = true;
        return;
      }
    }
    this._rows.push(row);
  };
  PropertyList.prototype.remove = function (key) {
    for (var i = this._rows.length - 1; i >= 0; i--) {
      var r = this._rows[i];
      if (typeof key === 'function' ? key({ key: r.key, value: r.value }) : this._is(r, key)) this._rows.splice(i, 1);
    }
  };
  PropertyList.prototype.clear = function () {
    this._rows.length = 0;
  };
  PropertyList.prototype.all = function () {
    return this._active().map(function (r) {
      return { key: r.key, value: r.value };
    });
  };
  PropertyList.prototype.toObject = function () {
    var out = {};
    this._active().forEach(function (r) {
      out[r.key] = r.value;
    });
    return out;
  };
  PropertyList.prototype.count = function () {
    return this._active().length;
  };
  PropertyList.prototype.each = function (fn) {
    this.all().forEach(fn);
  };
  PropertyList.prototype.map = function (fn) {
    return this.all().map(fn);
  };
  PropertyList.prototype.filter = function (fn) {
    return this.all().filter(fn);
  };
  PropertyList.prototype.find = function (fn) {
    return this.all().filter(fn)[0];
  };

  // ---- variables ----

  function scopeApi(scope, name) {
    return {
      name: name,
      get: function (key) {
        return native.get(scope, String(key));
      },
      set: function (key, value) {
        native.set(scope, String(key), str(value));
      },
      unset: function (key) {
        native.unset(scope, String(key));
      },
      has: function (key) {
        return native.get(scope, String(key)) !== undefined;
      },
      toObject: function () {
        return native.all(scope);
      },
      replaceIn: function (s) {
        return native.replaceIn(str(s), locals);
      },
    };
  }
  var environment = scopeApi('environment', input.environmentName || '');
  var collectionVariables = scopeApi('collection', '');
  var variables = {
    get: function (key) {
      key = String(key);
      if (Object.prototype.hasOwnProperty.call(locals, key)) return locals[key];
      var v = native.get('environment', key);
      return v !== undefined ? v : native.get('collection', key);
    },
    set: function (key, value) {
      locals[String(key)] = str(value);
    },
    unset: function (key) {
      delete locals[String(key)];
    },
    has: function (key) {
      return variables.get(key) !== undefined;
    },
    toObject: function () {
      var out = native.all('collection');
      var env = native.all('environment');
      Object.keys(env).forEach(function (k) {
        out[k] = env[k];
      });
      Object.keys(locals).forEach(function (k) {
        out[k] = locals[k];
      });
      return out;
    },
    replaceIn: function (s) {
      return native.replaceIn(str(s), locals);
    },
  };
  var unsupportedGlobals = function () {
    throw new Error('global variables are not supported; use pm.environment or pm.collectionVariables');
  };
  var globals = { get: unsupportedGlobals, set: unsupportedGlobals, unset: unsupportedGlobals, has: unsupportedGlobals };

  // ---- request ----

  var headerList = new PropertyList(req.headers, true);
  var queryList = new PropertyList(req.query, false);
  var url = {
    query: queryList,
    toString: function () {
      var qs = queryList
        ._active()
        .map(function (r) {
          return r.value === '' ? r.key : r.key + '=' + r.value;
        })
        .join('&');
      return qs ? req.url + (req.url.indexOf('?') < 0 ? '?' : '&') + qs : req.url;
    },
  };
  Object.defineProperty(url, 'raw', { get: url.toString, enumerable: true });
  var body = {
    mode: 'raw',
    toString: function () {
      return req.body;
    },
    isEmpty: function () {
      return req.body === '';
    },
    update: function (v) {
      body.raw = typeof v === 'string' ? v : v && v.raw !== undefined ? v.raw : JSON.stringify(v);
    },
  };
  Object.defineProperty(body, 'raw', {
    enumerable: true,
    get: function () {
      return req.body;
    },
    set: function (v) {
      req.body = str(v);
      if (req.bodyType === 'none') {
        try {
          JSON.parse(req.body);
          req.bodyType = 'json';
        } catch (e) {
          req.bodyType = 'text';
        }
      }
    },
  });
  var request = {
    headers: headerList,
    body: body,
    addHeader: function (item, value) {
      headerList.add(item, value);
    },
    upsertHeader: function (item, value) {
      headerList.upsert(item, value);
    },
    removeHeader: function (key) {
      headerList.remove(key);
    },
  };
  Object.defineProperty(request, 'method', {
    enumerable: true,
    get: function () {
      return req.method;
    },
    set: function (v) {
      req.method = String(v).toUpperCase();
    },
  });
  Object.defineProperty(request, 'url', {
    enumerable: true,
    get: function () {
      return url;
    },
    set: function (v) {
      req.url = String(v);
    },
  });

  // ---- expect (a subset of Chai's BDD API) ----

  function AssertionError(message) {
    this.name = 'AssertionError';
    this.message = message;
  }
  AssertionError.prototype = Object.create(Error.prototype);
  AssertionError.prototype.constructor = AssertionError;

  function typeOf(v) {
    if (v === null) return 'null';
    if (Array.isArray(v)) return 'array';
    if (v instanceof RegExp) return 'regexp';
    if (v instanceof Date) return 'date';
    return typeof v;
  }

  function deepEqual(a, b) {
    if (a === b) return true;
    if (typeof a === 'number' && typeof b === 'number') return a !== a && b !== b;
    if (typeOf(a) !== typeOf(b) || typeof a !== 'object' || a === null) return false;
    if (a instanceof Date) return a.getTime() === b.getTime();
    if (Array.isArray(a)) {
      if (a.length !== b.length) return false;
      for (var i = 0; i < a.length; i++) if (!deepEqual(a[i], b[i])) return false;
      return true;
    }
    var ka = Object.keys(a),
      kb = Object.keys(b);
    if (ka.length !== kb.length) return false;
    return ka.every(function (k) {
      return Object.prototype.hasOwnProperty.call(b, k) && deepEqual(a[k], b[k]);
    });
  }

  function Assertion(obj, message) {
    this._obj = obj;
    this._not = false;
    this._deep = false;
    this._message = message;
  }
  ['to', 'be', 'been', 'is', 'that', 'which', 'and', 'has', 'have', 'with', 'at', 'of', 'same', 'but', 'does', 'still'].forEach(
    function (word) {
      Object.defineProperty(Assertion.prototype, word, {
        get: function () {
          return this;
        },
      });
    },
  );
  Object.defineProperty(Assertion.prototype, 'not', {
    get: function () {
      this._not = !this._not;
      return this;
    },
  });
  Object.defineProperty(Assertion.prototype, 'deep', {
    get: function () {
      this._deep = true;
      return this;
    },
  });
  Assertion.prototype._assert = function (ok, what, expected, showExpected) {
    if (!!ok === this._not) {
      var msg =
        'expected ' + inspect(this._obj) + (this._not ? ' not ' : ' ') + 'to ' + what + (showExpected === false ? '' : ' ' + inspect(expected));
      throw new AssertionError(this._message ? this._message + ': ' + msg : msg);
    }
    return this;
  };
  function prop(name, test, what) {
    Object.defineProperty(Assertion.prototype, name, {
      get: function () {
        return this._assert(test(this._obj), what, undefined, false);
      },
    });
  }
  prop('ok', function (v) { return !!v; }, 'be truthy');
  prop('true', function (v) { return v === true; }, 'be true');
  prop('false', function (v) { return v === false; }, 'be false');
  prop('null', function (v) { return v === null; }, 'be null');
  prop('undefined', function (v) { return v === undefined; }, 'be undefined');
  prop('NaN', function (v) { return v !== v; }, 'be NaN');
  prop('exist', function (v) { return v !== null && v !== undefined; }, 'exist');
  prop('empty', function (v) {
    if (typeof v === 'string' || Array.isArray(v)) return v.length === 0;
    if (v && typeof v === 'object') return Object.keys(v).length === 0;
    return false;
  }, 'be empty');

  var A = Assertion.prototype;
  A.equal = A.equals = A.eq = function (v) {
    return this._assert(this._deep ? deepEqual(this._obj, v) : this._obj === v, this._deep ? 'deeply equal' : 'equal', v);
  };
  A.eql = A.eqls = function (v) {
    return this._assert(deepEqual(this._obj, v), 'deeply equal', v);
  };
  A.above = A.gt = A.greaterThan = function (n) {
    return this._assert(this._obj > n, 'be above', n);
  };
  A.least = A.gte = function (n) {
    return this._assert(this._obj >= n, 'be at least', n);
  };
  A.below = A.lt = A.lessThan = function (n) {
    return this._assert(this._obj < n, 'be below', n);
  };
  A.most = A.lte = function (n) {
    return this._assert(this._obj <= n, 'be at most', n);
  };
  A.within = function (lo, hi) {
    return this._assert(this._obj >= lo && this._obj <= hi, 'be within ' + lo + '..' + hi, undefined, false);
  };
  A.a = A.an = function (type) {
    return this._assert(typeOf(this._obj) === String(type).toLowerCase(), 'be a', type);
  };
  A.instanceof = A.instanceOf = function (ctor) {
    return this._assert(this._obj instanceof ctor, 'be an instance of ' + (ctor && ctor.name), undefined, false);
  };
  A.include = A.includes = A.contain = A.contains = function (v) {
    var o = this._obj,
      deep = this._deep,
      ok;
    if (typeof o === 'string') ok = o.indexOf(v) >= 0;
    else if (Array.isArray(o))
      ok = o.some(function (x) {
        return deep ? deepEqual(x, v) : x === v;
      });
    else if (o && typeof o === 'object' && v && typeof v === 'object')
      ok = Object.keys(v).every(function (k) {
        return deep ? deepEqual(o[k], v[k]) : o[k] === v[k];
      });
    else ok = false;
    return this._assert(ok, 'include', v);
  };
  A.string = function (s) {
    return this._assert(typeof this._obj === 'string' && this._obj.indexOf(s) >= 0, 'contain', s);
  };
  A.match = A.matches = function (re) {
    return this._assert(re.test(String(this._obj)), 'match', re);
  };
  A.oneOf = function (list) {
    var o = this._obj;
    return this._assert(
      list.some(function (x) {
        return deepEqual(x, o);
      }),
      'be one of',
      list,
    );
  };
  A.property = function (name, value) {
    var o = this._obj;
    var has = o !== null && o !== undefined && Object(o)[name] !== undefined;
    if (arguments.length > 1) {
      this._assert(has && (this._deep ? deepEqual(o[name], value) : o[name] === value), 'have property ' + inspect(name) + ' of', value);
    } else {
      this._assert(has, 'have property', name);
    }
    if (!this._not) this._obj = o[name];
    return this;
  };
  A.lengthOf = A.length = function (n) {
    var len = this._obj === null || this._obj === undefined ? undefined : this._obj.length;
    return this._assert(len === n, 'have length', n);
  };
  A.keys = A.key = function () {
    var want = Array.isArray(arguments[0]) ? arguments[0] : Array.prototype.slice.call(arguments);
    var have = this._obj && typeof this._obj === 'object' ? Object.keys(this._obj) : [];
    return this._assert(
      want.every(function (k) {
        return have.indexOf(k) >= 0;
      }),
      'have keys',
      want,
    );
  };
  A.members = function (list) {
    var o = this._obj;
    var ok =
      Array.isArray(o) &&
      o.length === list.length &&
      list.every(function (x) {
        return o.some(function (y) {
          return deepEqual(x, y);
        });
      });
    return this._assert(ok, 'have the same members as', list);
  };
  A.satisfy = A.satisfies = function (fn) {
    return this._assert(fn(this._obj), 'satisfy ' + (fn.name || 'the given function'), undefined, false);
  };

  function expect(v, message) {
    return new Assertion(v, message);
  }
  expect.fail = function (message) {
    throw new AssertionError(message || 'expect.fail()');
  };

  // ---- response ----

  var response;
  if (input.response) {
    var res = input.response;
    var resHeaders = new PropertyList(res.headers, true);
    var reason = String(res.status || '').replace(/^\d+\s*/, '');
    var parseJSON = function () {
      try {
        return JSON.parse(res.body);
      } catch (e) {
        throw new Error('response body is not valid JSON: ' + e.message);
      }
    };
    var responseAssert = function (negate) {
      var check = function (ok, msg) {
        if (!!ok === negate) throw new AssertionError('expected response ' + (negate ? 'not ' : '') + msg);
      };
      var be = {};
      [
        ['ok', function () { return res.code >= 200 && res.code < 300; }, 'to be 2xx (got ' + res.code + ')'],
        ['success', function () { return res.code >= 200 && res.code < 300; }, 'to be 2xx (got ' + res.code + ')'],
        ['clientError', function () { return res.code >= 400 && res.code < 500; }, 'to be 4xx (got ' + res.code + ')'],
        ['serverError', function () { return res.code >= 500; }, 'to be 5xx (got ' + res.code + ')'],
        ['json', function () { try { JSON.parse(res.body); return true; } catch (e) { return false; } }, 'to have a JSON body'],
      ].forEach(function (def) {
        Object.defineProperty(be, def[0], { get: function () { check(def[1](), def[2]); return true; } });
      });
      var to = {
        be: be,
        have: {
          status: function (code) {
            var ok = typeof code === 'number' ? res.code === code : reason.toLowerCase() === String(code).toLowerCase();
            check(ok, 'to have status ' + inspect(code) + ' but got ' + res.code + (reason ? ' ' + reason : ''));
          },
          header: function (key, value) {
            var actual = resHeaders.get(key);
            check(actual !== undefined && (value === undefined || actual === str(value)),
              'to have header ' + inspect(key) + (value === undefined ? '' : ' with value ' + inspect(value)) +
              (actual === undefined ? '' : ' (got ' + inspect(actual) + ')'));
          },
          body: function (text) {
            check(text === undefined ? res.body !== '' : res.body === str(text), text === undefined ? 'to have a body' : 'to have body ' + inspect(text));
          },
          jsonBody: function (path) {
            var data = parseJSON();
            if (path === undefined) return;
            var cur = data;
            String(path).split('.').forEach(function (p) { cur = cur === null || cur === undefined ? undefined : cur[p]; });
            check(cur !== undefined, 'to have JSON property ' + inspect(path));
          },
        },
      };
      if (!negate) Object.defineProperty(to, 'not', { get: function () { return responseAssert(true); } });
      return to;
    };
    response = {
      code: res.code,
      status: reason,
      responseTime: res.responseTime,
      responseSize: res.size,
      headers: resHeaders,
      text: function () {
        return res.body;
      },
      json: parseJSON,
    };
    Object.defineProperty(response, 'to', { get: function () { return responseAssert(false); } });
  }

  // ---- pm ----

  function test(name, fn) {
    var t = { name: String(name), passed: true, error: '' };
    try {
      if (typeof fn === 'function') fn();
    } catch (e) {
      t.passed = false;
      t.error = e && e.message !== undefined ? String(e.message) : String(e);
    }
    tests.push(t);
  }
  test.skip = function (name) {
    native.log('info', 'skipped test: ' + name);
  };

  var pm = {
    info: {
      eventName: input.phase === 'pre-request' ? 'prerequest' : 'test',
      requestName: input.requestName,
      requestId: input.requestId,
      iteration: 0,
      iterationCount: 1,
    },
    environment: environment,
    collectionVariables: collectionVariables,
    variables: variables,
    globals: globals,
    request: request,
    response: response,
    test: test,
    expect: expect,
    crypto: {
      rsaEncrypt: function (publicKey, data, options) {
        options = options || {};
        return native.rsaEncrypt(String(publicKey), str(data), options.padding || 'pkcs1', options.hash || 'sha256', options.encoding || 'base64');
      },
      rsaSign: function (privateKey, data, options) {
        options = options || {};
        return native.rsaSign(String(privateKey), str(data), options.padding || 'pkcs1', options.hash || 'sha256', options.encoding || 'base64');
      },
    },
    sendRequest: function () {
      throw new Error('pm.sendRequest is not supported');
    },
  };

  // ---- libraries ----

  var libs = {};
  function loadLib(name) {
    if (!libs[name]) libs[name] = native.loadLib(name);
    return libs[name];
  }
  var modules = {
    'crypto-js': function () { return loadLib('crypto-js'); },
    lodash: function () { return loadLib('lodash'); },
    moment: function () { return loadLib('moment'); },
    uuid: function () { return { v4: native.uuid }; },
    atob: function () { return native.atob; },
    btoa: function () { return native.btoa; },
  };
  function require(name) {
    if (!Object.prototype.hasOwnProperty.call(modules, name)) {
      throw new Error("module '" + name + "' is not available; available modules: " + Object.keys(modules).join(', '));
    }
    return modules[name]();
  }

  function lazyGlobal(name, load) {
    var loading = false;
    Object.defineProperty(global, name, {
      configurable: true,
      enumerable: false,
      get: function () {
        // lodash reads the global _ while it loads (for noConflict).
        if (loading) return undefined;
        loading = true;
        var v;
        try {
          v = load();
        } finally {
          loading = false;
        }
        Object.defineProperty(global, name, { value: v, writable: true, configurable: true });
        return v;
      },
      set: function (v) {
        Object.defineProperty(global, name, { value: v, writable: true, configurable: true });
      },
    });
  }

  global.pm = pm;
  global.console = consoleObj;
  global.require = require;
  global.atob = native.atob;
  global.btoa = native.btoa;
  // crypto-js uses crypto.getRandomValues for random keys, salts and IVs.
  global.crypto = {
    getRandomValues: function (arr) {
      var bytes = native.randomBytes(arr.byteLength);
      var view = new Uint8Array(arr.buffer, arr.byteOffset, arr.byteLength);
      for (var i = 0; i < bytes.length; i++) view[i] = bytes[i];
      return arr;
    },
    randomUUID: native.uuid,
  };
  lazyGlobal('CryptoJS', modules['crypto-js']);
  lazyGlobal('_', modules.lodash);
  lazyGlobal('moment', modules.moment);

  // Legacy Postman script globals.
  global.tests = {};
  global.postman = {
    setEnvironmentVariable: environment.set,
    getEnvironmentVariable: environment.get,
    clearEnvironmentVariable: environment.unset,
  };
  if (input.response) {
    global.responseBody = input.response.body;
    global.responseCode = { code: input.response.code, name: response.status };
    global.responseTime = input.response.responseTime;
    global.responseHeaders = response.headers.toObject();
  }

  global.__finish = function () {
    Object.keys(global.tests).forEach(function (name) {
      tests.push({ name: name, passed: !!global.tests[name], error: global.tests[name] ? '' : 'test returned false' });
    });
    return JSON.stringify({ request: req, locals: locals, tests: tests });
  };
})(this);
