(() => {
  var __defProp = Object.defineProperty;
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };
  var __async = (__this, __arguments, generator) => {
    return new Promise((resolve, reject) => {
      var fulfilled = (value) => {
        try {
          step(generator.next(value));
        } catch (e) {
          reject(e);
        }
      };
      var rejected = (value) => {
        try {
          step(generator.throw(value));
        } catch (e) {
          reject(e);
        }
      };
      var step = (x) => x.done ? resolve(x.value) : Promise.resolve(x.value).then(fulfilled, rejected);
      step((generator = generator.apply(__this, __arguments)).next());
    });
  };

  // src/webauthn-json/index.ts
  var webauthn_json_exports = {};
  __export(webauthn_json_exports, {
    create: () => create,
    get: () => get,
    schema: () => schema,
    supported: () => supported
  });

  // src/webauthn-json/base64url.ts
  function base64urlToBuffer(baseurl64String) {
    const padding = "==".slice(0, (4 - baseurl64String.length % 4) % 4);
    const base64String = baseurl64String.replace(/-/g, "+").replace(/_/g, "/") + padding;
    const str = atob(base64String);
    const buffer = new ArrayBuffer(str.length);
    const byteView = new Uint8Array(buffer);
    for (let i = 0; i < str.length; i++) {
      byteView[i] = str.charCodeAt(i);
    }
    return buffer;
  }
  function bufferToBase64url(buffer) {
    const byteView = new Uint8Array(buffer);
    let str = "";
    for (const charCode of byteView) {
      str += String.fromCharCode(charCode);
    }
    const base64String = btoa(str);
    const base64urlString = base64String.replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
    return base64urlString;
  }

  // src/webauthn-json/convert.ts
  var copyValue = "copy";
  var convertValue = "convert";
  function convert(conversionFn, schema2, input) {
    if (schema2 === copyValue) {
      return input;
    }
    if (schema2 === convertValue) {
      return conversionFn(input);
    }
    if (schema2 instanceof Array) {
      return input.map((v) => convert(conversionFn, schema2[0], v));
    }
    if (schema2 instanceof Object) {
      const output = {};
      for (const [key, schemaField] of Object.entries(schema2)) {
        if (schemaField.derive) {
          const v = schemaField.derive(input);
          if (v !== void 0) {
            input[key] = v;
          }
        }
        if (!(key in input)) {
          if (schemaField.required) {
            throw new Error(`Missing key: ${key}`);
          }
          continue;
        }
        if (input[key] == null) {
          output[key] = null;
          continue;
        }
        output[key] = convert(conversionFn, schemaField.schema, input[key]);
      }
      return output;
    }
  }
  function derived(schema2, derive) {
    return {
      required: true,
      schema: schema2,
      derive
    };
  }
  function required(schema2) {
    return {
      required: true,
      schema: schema2
    };
  }
  function optional(schema2) {
    return {
      required: false,
      schema: schema2
    };
  }

  // src/webauthn-json/basic/schema.ts
  var publicKeyCredentialDescriptorSchema = {
    type: required(copyValue),
    id: required(convertValue),
    transports: optional(copyValue)
  };
  var simplifiedExtensionsSchema = {
    appid: optional(copyValue),
    appidExclude: optional(copyValue),
    credProps: optional(copyValue)
  };
  var simplifiedClientExtensionResultsSchema = {
    appid: optional(copyValue),
    appidExclude: optional(copyValue),
    credProps: optional(copyValue)
  };
  var credentialCreationOptions = {
    publicKey: required({
      rp: required(copyValue),
      user: required({
        id: required(convertValue),
        name: required(copyValue),
        displayName: required(copyValue)
      }),
      challenge: required(convertValue),
      pubKeyCredParams: required(copyValue),
      timeout: optional(copyValue),
      excludeCredentials: optional([publicKeyCredentialDescriptorSchema]),
      authenticatorSelection: optional(copyValue),
      attestation: optional(copyValue),
      extensions: optional(simplifiedExtensionsSchema)
    }),
    signal: optional(copyValue)
  };
  var publicKeyCredentialWithAttestation = {
    type: required(copyValue),
    id: required(copyValue),
    rawId: required(convertValue),
    authenticatorAttachment: optional(copyValue),
    response: required({
      clientDataJSON: required(convertValue),
      attestationObject: required(convertValue),
      transports: derived(copyValue, (response) => {
        var _a;
        return ((_a = response.getTransports) == null ? void 0 : _a.call(response)) || [];
      })
    }),
    clientExtensionResults: derived(simplifiedClientExtensionResultsSchema, (pkc) => pkc.getClientExtensionResults())
  };
  var credentialRequestOptions = {
    mediation: optional(copyValue),
    publicKey: required({
      challenge: required(convertValue),
      timeout: optional(copyValue),
      rpId: optional(copyValue),
      allowCredentials: optional([publicKeyCredentialDescriptorSchema]),
      userVerification: optional(copyValue),
      extensions: optional(simplifiedExtensionsSchema)
    }),
    signal: optional(copyValue)
  };
  var publicKeyCredentialWithAssertion = {
    type: required(copyValue),
    id: required(copyValue),
    rawId: required(convertValue),
    authenticatorAttachment: optional(copyValue),
    response: required({
      clientDataJSON: required(convertValue),
      authenticatorData: required(convertValue),
      signature: required(convertValue),
      userHandle: required(convertValue)
    }),
    clientExtensionResults: derived(simplifiedClientExtensionResultsSchema, (pkc) => pkc.getClientExtensionResults())
  };
  var schema = {
    credentialCreationOptions,
    publicKeyCredentialWithAttestation,
    credentialRequestOptions,
    publicKeyCredentialWithAssertion
  };

  // src/webauthn-json/basic/api.ts
  function createRequestFromJSON(requestJSON) {
    return convert(base64urlToBuffer, credentialCreationOptions, requestJSON);
  }
  function createResponseToJSON(credential) {
    return convert(bufferToBase64url, publicKeyCredentialWithAttestation, credential);
  }
  function create(requestJSON) {
    return __async(this, null, function* () {
      const credential = yield navigator.credentials.create(createRequestFromJSON(requestJSON));
      return createResponseToJSON(credential);
    });
  }
  function getRequestFromJSON(requestJSON) {
    return convert(base64urlToBuffer, credentialRequestOptions, requestJSON);
  }
  function getResponseToJSON(credential) {
    return convert(bufferToBase64url, publicKeyCredentialWithAssertion, credential);
  }
  function get(requestJSON) {
    return __async(this, null, function* () {
      const credential = yield navigator.credentials.get(getRequestFromJSON(requestJSON));
      return getResponseToJSON(credential);
    });
  }

  // src/webauthn-json/basic/supported.ts
  function supported() {
    return !!(navigator.credentials && navigator.credentials.create && navigator.credentials.get && window.PublicKeyCredential);
  }

  // src/webauthn-json/browser-global.ts
  globalThis.webauthnJSON = webauthn_json_exports;
})();
//# sourceMappingURL=webauthn-json.browser-global.js.map
