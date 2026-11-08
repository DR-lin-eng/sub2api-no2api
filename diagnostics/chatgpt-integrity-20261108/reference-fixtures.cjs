// Offline oracle: execute the user-supplied, exact lXEa source with synthetic
// challenges. No account, browser, network, SDK download or production request.
const fs = require('node:fs');
const vm = require('node:vm');
const crypto = require('node:crypto');
const path = require('node:path');

async function main() {
  const source = fs.readFileSync(process.argv[2], 'utf8');
  const exports = {};
  const loader = () => ({ A: () => '11111111-1111-4111-8111-111111111111' });
  loader.d = (target, values) => {
    for (const [name, get] of Object.entries(values)) Object.defineProperty(target, name, { get });
  };
  const context = vm.createContext({
    window: { navigator: { language: 'en' }, Math }, document: { scripts: [], documentElement: { getAttribute: () => null } },
    navigator: Object.create({ userAgent: 'fixture-agent', language: 'en', languages: ['en'], hardwareConcurrency: 2 }),
    screen: { width: 0, height: 0 }, performance: { now: () => 1, timeOrigin: 1700000000000 },
    URLSearchParams, TextEncoder, atob, btoa, setTimeout,
  });
  vm.runInContext(source, context, { timeout: 1000 }).lXEa({}, exports, loader);
  const output = {
    sourceSHA256: crypto.createHash('sha256').update(source).digest('hex'),
    fixtures: [],
  };
  const programs = {
    resolve: [[3, 'fixture-ok']],
    latin1: [[3, 'éÿ']],
    arithmetic: [[2,40,7],[2,41,3],[33,42,40,41],[35,43,42,41],[29,44,41,40],[5,40,41],[27,40,41],[2,45,0],[35,46,40,45],[2,50,[]],[5,50,42],[5,50,43],[5,50,44],[5,50,46],[15,51,50],[7,3,51]],
    strings: [[2,40,'hello'],[2,41,'key'],[1,40,41],[1,40,41],[19,40],[18,40],[8,42,40],[7,3,42]],
    arrayRemoval: [[2,40,[1,2,3]],[2,41,99],[27,40,41],[15,42,40],[7,3,42]],
    parse: [[2,40,'{"value":42}'],[14,41,40],[2,42,'value'],[6,43,41,42],[7,3,43]],
    conditionals: [[2,40,1],[2,41,1],[20,40,41,2,42,'equal'],[2,43,5],[2,44,2],[21,40,43,44,2,45,'threshold'],[23,42,2,46,'defined'],[13,47,999],[23,47,2,48,'caught'],[2,50,[]],[5,50,42],[5,50,45],[5,50,46],[5,50,48],[15,51,50],[7,3,51]],
    nestedQueue: [[22,40,[[2,41,'from-subqueue']]],[7,3,41]],
    nestedError: [[22,40,[[7,999]]],[23,40,2,41,'caught-subqueue'],[7,3,41]],
    closureAndPromise: [[30,40,53,[51,52],[[33,53,51,52]]],[2,41,6],[2,42,7],[17,43,40,41,42],[34,44,43],[7,3,44]],
    closureWithoutParams: [[30,40,41,[[2,41,'closure']]],[17,42,40],[7,3,42]],
    mapAccess: [[12,40],[2,41,'get'],[24,42,40,41],[2,43,16],[17,44,42,43],[2,45,'gAAAAAC'],[2,46,'startsWith'],[24,47,44,46],[17,48,47,45],[7,3,48]],
    navigator: [[2,40,'navigator'],[6,41,10,40],[2,42,'language'],[6,43,41,42],[7,3,43]],
    boundMath: [[2,40,'Math'],[6,41,10,40],[2,42,'round'],[24,43,41,42],[2,44,2.6],[17,45,43,44],[7,3,45]],
    scriptsAndNoops: [[2,40,'c/[^/]*/_'],[11,41,40],[25],[26],[28],[31],[32],[15,42,41],[7,3,42]],
    timeoutCount: [[25],[26],[28]],
    reject: [[4,'fixture-rejected']],
  };
  for (const [name, program] of Object.entries(programs)) {
    let rejected = false, token;
    try {
      const result = await exports.$A(async key => {
        const data = JSON.stringify(program);
        let dx = '';
        for (let n = 0; n < data.length; n++) dx += String.fromCharCode(data.charCodeAt(n) ^ key.charCodeAt(n % key.length));
        return { token: 'fixture-requirements', turnstile: { required: true, dx: btoa(dx) } };
      });
      token = result.turnstileToken;
    } catch (error) { rejected = true; token = String(error); }
    output.fixtures.push({ name, program, token, rejected });
  }
  // The hash is taken from the actual source expression, not a second copy of
  // the Go implementation. Astral characters exercise UTF-16 code units.
  const expression = source.match(/\(function\(e\)\{let t=0x811c9dc5;.*?padStart\(8,"0"\)\}\)/);
  if (!expression) throw Error('reference hash expression not found');
  const hash = vm.runInNewContext(expression[0]);
  output.hashes = ['', 'fixture', 'seed🌈漢字gAAAAAC', 'é\u2028\u2029'].map(input => ({ input, hex: hash(input) }));
  const destination = path.resolve(__dirname, '../../backend/internal/modules/chatgptimages/testdata');
  fs.mkdirSync(destination, { recursive: true });
  fs.writeFileSync(path.join(destination, 'integrity_vm_golden.json'), JSON.stringify(output, null, 2) + '\n');
  console.log(JSON.stringify({ sourceSHA256: output.sourceSHA256, fixtures: output.fixtures.length, hashes: output.hashes.length }));
}
main().catch(error => { console.error(error); process.exitCode = 1; });
