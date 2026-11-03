// Ready-made script snippets for the script editor (see internal/scripting).

export type ScriptPhase = 'pre' | 'post';

export interface ScriptSnippet {
  phase: ScriptPhase;
  label: string;
  code: string;
}

export const SCRIPT_PLACEHOLDERS: Record<ScriptPhase, string> = {
  pre: `// Example: sign the body with HMAC-SHA256
const body = pm.variables.replaceIn(pm.request.body.raw);
pm.variables.set('signature', CryptoJS.HmacSHA256(body, pm.environment.get('api_secret')).toString());`,
  post: `// Example
pm.test('Status code is 200', () => pm.response.to.have.status(200));
pm.environment.set('token', pm.response.json().token);`,
};

export const SCRIPT_SNIPPETS: ScriptSnippet[] = [
  {
    phase: 'pre',
    label: 'HMAC-SHA256 signature header',
    code: `// Sign the request body (with variables resolved) using a secret.
const body = pm.variables.replaceIn(pm.request.body.raw);
const signature = CryptoJS.HmacSHA256(body, pm.environment.get('api_secret')).toString(CryptoJS.enc.Hex);
pm.request.headers.upsert({ key: 'X-Signature', value: signature });`,
  },
  {
    phase: 'pre',
    label: 'Timestamp and nonce variables',
    code: `// Use as {{timestamp}} and {{nonce}} in the URL, headers or body.
pm.variables.set('timestamp', Date.now().toString());
pm.variables.set('nonce', require('uuid').v4());`,
  },
  {
    phase: 'pre',
    label: 'Hash a value (SHA-256 / MD5)',
    code: `const text = pm.variables.replaceIn('{{merchant_id}}{{timestamp}}');
pm.variables.set('sha256', CryptoJS.SHA256(text).toString());
pm.variables.set('md5', CryptoJS.MD5(text).toString());`,
  },
  {
    phase: 'pre',
    label: 'AES encrypt the body',
    code: `// AES-CBC with PKCS7 padding. Key: 16, 24 or 32 characters; IV: 16 characters.
const key = CryptoJS.enc.Utf8.parse(pm.environment.get('aes_key'));
const iv = CryptoJS.enc.Utf8.parse(pm.environment.get('aes_iv'));
const plain = pm.variables.replaceIn(pm.request.body.raw);
const encrypted = CryptoJS.AES.encrypt(plain, key, { iv, mode: CryptoJS.mode.CBC, padding: CryptoJS.pad.Pkcs7 });
pm.request.body.raw = JSON.stringify({ data: encrypted.toString() });`,
  },
  {
    phase: 'pre',
    label: 'RSA encrypt a value',
    code: `// The public key is a PEM ("-----BEGIN PUBLIC KEY-----...") stored in a variable.
const encrypted = pm.crypto.rsaEncrypt(pm.environment.get('public_key'), pm.variables.replaceIn('{{card_number}}'));
pm.variables.set('encrypted_card', encrypted);`,
  },
  {
    phase: 'pre',
    label: 'RSA sign the body (SHA256withRSA)',
    code: `const body = pm.variables.replaceIn(pm.request.body.raw);
const signature = pm.crypto.rsaSign(pm.environment.get('private_key'), body, { hash: 'sha256' });
pm.request.headers.upsert({ key: 'X-Signature', value: signature });`,
  },
  {
    phase: 'pre',
    label: 'Basic auth header from variables',
    code: `const credentials = pm.variables.replaceIn('{{username}}:{{password}}');
pm.request.headers.upsert({ key: 'Authorization', value: 'Basic ' + btoa(credentials) });`,
  },
  {
    phase: 'post',
    label: 'Status code is 200',
    code: `pm.test('Status code is 200', () => {
  pm.response.to.have.status(200);
});`,
  },
  {
    phase: 'post',
    label: 'Check a JSON value',
    code: `pm.test('Response has the expected data', () => {
  const data = pm.response.json();
  pm.expect(data).to.have.property('id');
  pm.expect(data.status).to.eql('success');
});`,
  },
  {
    phase: 'post',
    label: 'Save a token to the environment',
    code: `// Later requests can use {{token}}; the value is saved to the selected environment.
const data = pm.response.json();
pm.environment.set('token', data.access_token);`,
  },
  {
    phase: 'post',
    label: 'Response time is below 500 ms',
    code: `pm.test('Response time is below 500 ms', () => {
  pm.expect(pm.response.responseTime).to.be.below(500);
});`,
  },
  {
    phase: 'post',
    label: 'AES decrypt a response field',
    code: `const key = CryptoJS.enc.Utf8.parse(pm.environment.get('aes_key'));
const iv = CryptoJS.enc.Utf8.parse(pm.environment.get('aes_iv'));
const decrypted = CryptoJS.AES.decrypt(pm.response.json().data, key, { iv }).toString(CryptoJS.enc.Utf8);
console.log('decrypted', decrypted);`,
  },
];
