// Zero-dependency GitHub Action that deploys a static directory to Alibaba Cloud
// ESA Pages. It performs the same public API sequence as the official esa-cli:
// create an assets code version, upload the zip to the returned OSS form target,
// wait for the version to become available, then publish it to an environment.

const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const zlib = require('node:zlib');

const API_VERSION = '2024-09-10';
const DEFAULT_ENDPOINT = 'esa.cn-hangzhou.aliyuncs.com';
const DEFAULT_REGION = 'cn-hangzhou';
const READY_TIMEOUT_MS = 5 * 60 * 1000;
const READY_INTERVAL_MS = 1000;
const UPLOAD_ATTEMPTS = 3;

const CRC_TABLE = (() => {
  const table = new Int32Array(256);
  for (let i = 0; i < 256; i += 1) {
    let value = i;
    for (let bit = 0; bit < 8; bit += 1) {
      value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
    }
    table[i] = value;
  }
  return table;
})();

function crc32(buffer) {
  let value = -1;
  for (let i = 0; i < buffer.length; i += 1) {
    value = CRC_TABLE[(value ^ buffer[i]) & 0xff] ^ (value >>> 8);
  }
  return (value ^ -1) >>> 0;
}

function percentEncode(value) {
  return encodeURIComponent(String(value))
    .replace(/!/g, '%21')
    .replace(/'/g, '%27')
    .replace(/\(/g, '%28')
    .replace(/\)/g, '%29')
    .replace(/\*/g, '%2A')
    .replace(/%7E/g, '~');
}

function collectFiles(root, relative = '') {
  const entries = fs.readdirSync(path.join(root, relative), { withFileTypes: true });
  const files = [];
  for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
    const next = relative ? `${relative}/${entry.name}` : entry.name;
    if (entry.isDirectory()) {
      files.push(...collectFiles(root, next));
    } else if (entry.isFile()) {
      files.push(next);
    }
  }
  return files;
}

// Builds a deterministic zip whose entries all live under assets/, matching the
// layout produced by esa-cli for static Pages projects.
function buildZip(root) {
  const entries = collectFiles(root).map((relative) => {
    const data = fs.readFileSync(path.join(root, relative));
    return {
      name: `assets/${relative}`,
      data,
      compressed: zlib.deflateRawSync(data),
    };
  });
  if (entries.length === 0) {
    throw new Error(`No files found under ${root}`);
  }

  const localParts = [];
  const centralParts = [];
  let offset = 0;
  for (const entry of entries) {
    const name = Buffer.from(entry.name, 'utf8');
    const crc = crc32(entry.data);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0x0800, 6);
    local.writeUInt16LE(8, 8);
    local.writeUInt16LE(0, 10);
    local.writeUInt16LE(0x21, 12);
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(entry.compressed.length, 18);
    local.writeUInt32LE(entry.data.length, 22);
    local.writeUInt16LE(name.length, 26);
    local.writeUInt16LE(0, 28);
    localParts.push(local, name, entry.compressed);

    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(20, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt16LE(0x0800, 8);
    central.writeUInt16LE(8, 10);
    central.writeUInt16LE(0, 12);
    central.writeUInt16LE(0x21, 14);
    central.writeUInt32LE(crc, 16);
    central.writeUInt32LE(entry.compressed.length, 20);
    central.writeUInt32LE(entry.data.length, 24);
    central.writeUInt16LE(name.length, 28);
    central.writeUInt16LE(0, 30);
    central.writeUInt16LE(0, 32);
    central.writeUInt16LE(0, 34);
    central.writeUInt16LE(0, 36);
    central.writeUInt32LE(0, 38);
    central.writeUInt32LE(offset, 42);
    centralParts.push(central, name);
    offset += local.length + name.length + entry.compressed.length;
  }

  const centralDirectory = Buffer.concat(centralParts);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(0, 4);
  end.writeUInt16LE(0, 6);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(centralDirectory.length, 12);
  end.writeUInt32LE(offset, 16);
  end.writeUInt16LE(0, 20);

  return {
    buffer: Buffer.concat([...localParts, centralDirectory, end]),
    fileCount: entries.length,
  };
}

function createSigner({ accessKeyId, accessKeySecret, endpoint, region, securityToken }) {
  const common = () => ({
    AccessKeyId: accessKeyId,
    Format: 'JSON',
    RegionId: region,
    SignatureMethod: 'HMAC-SHA1',
    SignatureNonce: crypto.randomUUID(),
    SignatureVersion: '1.0',
    Timestamp: new Date().toISOString().replace(/\.\d{3}Z$/, 'Z'),
    Version: API_VERSION,
    ...(securityToken ? { SecurityToken: securityToken } : {}),
  });

  return async function callApi(action, params = {}) {
    const all = { ...common(), Action: action, ...params };
    const canonical = Object.keys(all)
      .sort()
      .map((key) => `${percentEncode(key)}=${percentEncode(all[key])}`)
      .join('&');
    const stringToSign = `POST&${percentEncode('/')}&${percentEncode(canonical)}`;
    const signature = crypto
      .createHmac('sha1', `${accessKeySecret}&`)
      .update(stringToSign)
      .digest('base64');
    const body = `${canonical}&Signature=${percentEncode(signature)}`;

    const response = await fetch(`https://${endpoint}/`, {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body,
    });
    const text = await response.text();
    let payload;
    try {
      payload = text ? JSON.parse(text) : {};
    } catch {
      throw new Error(`ESA API ${action} returned non-JSON response (HTTP ${response.status})`);
    }
    if (!response.ok || (payload.Code && String(payload.Code) !== '200')) {
      const code = payload.Code || `HTTP ${response.status}`;
      const message = payload.Message || 'unknown error';
      throw new Error(`ESA API ${action} failed: ${code}: ${message}`);
    }
    return payload;
  };
}

async function uploadToOss(config, buffer) {
  if (!config || !config.OSSAccessKeyId || !config.Signature || !config.Url || !config.Key || !config.Policy) {
    throw new Error('ESA API did not return a complete OSS upload configuration');
  }
  for (let attempt = 1; attempt <= UPLOAD_ATTEMPTS; attempt += 1) {
    const form = new FormData();
    form.append('OSSAccessKeyId', config.OSSAccessKeyId);
    form.append('Signature', config.Signature);
    if (config.XOssSecurityToken) {
      form.append('x-oss-security-token', config.XOssSecurityToken);
    }
    form.append('policy', config.Policy);
    form.append('key', config.Key);
    form.append('file', new Blob([buffer], { type: 'application/zip' }), 'site.zip');
    try {
      const response = await fetch(config.Url, { method: 'POST', body: form });
      if (response.status === 200 || response.status === 204) {
        return;
      }
      if (attempt === UPLOAD_ATTEMPTS) {
        throw new Error(`OSS upload failed with HTTP ${response.status}`);
      }
    } catch (error) {
      if (attempt === UPLOAD_ATTEMPTS) {
        throw error;
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 2000));
  }
}

function setOutput(name, value) {
  if (process.env.GITHUB_OUTPUT) {
    fs.appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`);
  }
}

async function deploy(options) {
  const {
    project,
    sitePath,
    environment,
    description,
    accessKeyId,
    accessKeySecret,
    securityToken,
    endpoint,
    notFoundStrategy,
  } = options;

  const startedAt = Date.now();
  const elapsed = () => `${((Date.now() - startedAt) / 1000).toFixed(1)}s`;

  const resolved = path.resolve(process.env.GITHUB_WORKSPACE || process.cwd(), sitePath);
  if (!fs.existsSync(resolved) || !fs.statSync(resolved).isDirectory()) {
    throw new Error(`Site directory does not exist: ${resolved}`);
  }

  const { buffer, fileCount } = buildZip(resolved);
  console.log(`Packed ${fileCount} files (${(buffer.length / 1024 / 1024).toFixed(2)} MiB) in ${elapsed()}`);

  const callApi = createSigner({
    accessKeyId,
    accessKeySecret,
    endpoint,
    region: process.env.ESA_REGION || DEFAULT_REGION,
    securityToken,
  });

  const version = await callApi('CreateRoutineWithAssetsCodeVersion', {
    Name: project,
    CodeDescription: description || '',
    ExtraInfo: JSON.stringify({ Source: 'GitHubActions' }),
    DeployEnv: environment,
    // Ask Pages to serve the site's own 404.html instead of the default error page.
    ...(notFoundStrategy ? { ConfOptions: JSON.stringify({ NotFoundStrategy: notFoundStrategy }) } : {}),
  });
  if (!version.CodeVersion) {
    throw new Error('ESA API did not return a CodeVersion');
  }
  console.log(`Created code version ${version.CodeVersion} in ${elapsed()}`);

  await uploadToOss(version.OssPostConfig, buffer);
  console.log(`Uploaded assets in ${elapsed()}`);

  const deadline = Date.now() + READY_TIMEOUT_MS;
  let status = '';
  for (;;) {
    const info = await callApi('GetRoutineCodeVersionInfo', {
      Name: project,
      CodeVersion: version.CodeVersion,
    });
    status = String(info.Status || '').toLowerCase();
    if (status === 'available') {
      break;
    }
    if (status && status !== 'init') {
      throw new Error(`Code version ${version.CodeVersion} is ${status}`);
    }
    if (Date.now() > deadline) {
      throw new Error(`Timed out waiting for code version ${version.CodeVersion}`);
    }
    await new Promise((resolve) => setTimeout(resolve, READY_INTERVAL_MS));
  }
  console.log(`Code version ${version.CodeVersion} is ready in ${elapsed()}`);

  const deployment = await callApi('CreateRoutineCodeDeployment', {
    Name: project,
    Env: environment,
    Strategy: 'percentage',
    CodeVersions: JSON.stringify([{ Percentage: 100, CodeVersion: version.CodeVersion }]),
  });
  console.log(`Published to ${environment} in ${elapsed()}`);

  setOutput('code-version', version.CodeVersion);
  if (deployment.DeploymentId) {
    setOutput('deployment-id', deployment.DeploymentId);
  }
  return { codeVersion: version.CodeVersion, deploymentId: deployment.DeploymentId };
}

async function main() {
  const accessKeyId = process.env.ESA_ACCESS_KEY_ID;
  const accessKeySecret = process.env.ESA_ACCESS_KEY_SECRET;
  if (!accessKeyId || !accessKeySecret) {
    throw new Error('ESA_ACCESS_KEY_ID and ESA_ACCESS_KEY_SECRET must be set');
  }
  const project = process.env.INPUT_PROJECT;
  const sitePath = process.env.INPUT_PATH;
  if (!project || !sitePath) {
    throw new Error('Both project and path inputs are required');
  }
  await deploy({
    project,
    sitePath,
    environment: process.env.INPUT_ENVIRONMENT || 'production',
    description: process.env.INPUT_DESCRIPTION || '',
    accessKeyId,
    accessKeySecret,
    securityToken: process.env.ESA_SECURITY_TOKEN,
    endpoint: process.env.ESA_ENDPOINT || DEFAULT_ENDPOINT,
    notFoundStrategy: process.env.INPUT_NOT_FOUND_STRATEGY || '',
  });
}

module.exports = { buildZip, percentEncode, crc32, deploy };

if (require.main === module) {
  main().catch((error) => {
    // Never surface request parameters or credentials in the failure message.
    console.error(error.message);
    process.exitCode = 1;
  });
}
