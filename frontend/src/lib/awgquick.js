// awg-quick <-> structured WireGuard field helpers. The structured fields are the
// source of truth used to bring the tunnel up; the raw text is an editing view that
// syncs both ways. parseAwgQuick returns only the keys it found (so deleting a stray
// line never silently wipes an unrelated field); buildAwgQuick emits only non-empty
// values as an [Interface]/[Peer] INI.

const IFACE_KEYS = {
  privatekey: 'privateKey',
  address: 'addresses',
  dns: 'dns',
  mtu: 'mtu',
  jc: 'jc',
  jmin: 'jmin',
  jmax: 'jmax',
  s1: 's1',
  s2: 's2',
  s3: 's3',
  s4: 's4',
  h1: 'h1',
  h2: 'h2',
  h3: 'h3',
  h4: 'h4',
};
const PEER_KEYS = {
  publickey: 'publicKey',
  presharedkey: 'presharedKey',
  allowedips: 'allowedIps',
  endpoint: 'endpoint',
};

export function parseAwgQuick(raw) {
  const out = {};
  let section = '';
  for (let line of String(raw ?? '').split('\n')) {
    line = line.trim();
    if (!line || line.startsWith('#')) continue;
    if (line.startsWith('[') && line.endsWith(']')) {
      section = line.slice(1, -1).trim().toLowerCase();
      continue;
    }
    const eq = line.indexOf('=');
    if (eq < 0) continue;
    const key = line.slice(0, eq).trim().toLowerCase();
    const val = line.slice(eq + 1).trim();
    const map = section === 'interface' ? IFACE_KEYS : section === 'peer' ? PEER_KEYS : null;
    if (!map || !(key in map)) continue;
    const field = map[key];
    out[field] = field === 'mtu' ? parseInt(val, 10) || 1280 : val;
  }
  return out;
}

export function buildAwgQuick(wg) {
  const w = wg ?? {};
  const line = (k, v) =>
    v !== undefined && v !== null && String(v).trim() !== '' ? `${k} = ${String(v).trim()}` : null;
  const iface = [
    '[Interface]',
    line('PrivateKey', w.privateKey),
    line('Address', w.addresses),
    line('DNS', w.dns),
    line('MTU', w.mtu),
    line('Jc', w.jc),
    line('Jmin', w.jmin),
    line('Jmax', w.jmax),
    line('S1', w.s1),
    line('S2', w.s2),
    line('S3', w.s3),
    line('S4', w.s4),
    line('H1', w.h1),
    line('H2', w.h2),
    line('H3', w.h3),
    line('H4', w.h4),
  ].filter(Boolean);
  const peer = [
    '[Peer]',
    line('PublicKey', w.publicKey),
    line('PresharedKey', w.presharedKey),
    line('AllowedIPs', w.allowedIps),
    line('Endpoint', w.endpoint),
  ].filter(Boolean);
  return `${iface.join('\n')}\n\n${peer.join('\n')}\n`;
}
