// Protocol VM from the 20261107 lXEa module. This is a bounded environment
// model: it does not claim a DOM, a browser session or Apple device evidence.
(function (now, uuid, btoa, atob) {
    "use strict";
    const maxValue = 262144, maxArray = 4096;
    let registers, count = 0, steps = 0, settled = false, expire, chain = Promise.resolve();
    function budget() { const e = new Error("verification budget"); e.name = "BudgetError"; throw e; }
    function check(value) {
        if (typeof value === "string" && value.length > maxValue) budget();
        if (Array.isArray(value) && value.length > maxArray) budget();
        return value;
    }
    function put(key, value) {
        if (registers.size >= 1024 && !registers.has(key)) budget();
        registers.set(key, check(value));
    }
    function safeJSON(value) {
        let nodes = 0, size = 0;
        function visit(v, depth, path) {
            if (++nodes > 16384 || depth > 32) budget();
            if (typeof v === "string") size += v.length * 6 + 2;
            else if (v && typeof v === "object") {
                if (path.indexOf(v) >= 0) throw new TypeError("Converting circular structure to JSON");
                const keys = Object.keys(v);
                if (keys.length > maxArray) budget();
                path.push(v);
                for (const key of keys) { size += key.length * 6 + 4; visit(v[key], depth + 1, path); }
                path.pop();
            } else size += 32;
            if (size > maxValue) budget();
        }
        visit(value, 0, []);
        return check(JSON.stringify(value));
    }
    function property(object, name, bind) {
        name = String(name);
        // The challenge gets data/functions from the modeled window only.
        // Constructor/prototype traversal must not expose the JS global host.
        if (["constructor", "__proto__", "prototype", "eval", "Function"].indexOf(name) >= 0) budget();
        const value = object[name];
        if (typeof value !== "function") return value;
        return function () {
            const receiver = bind ? object : this;
            const args = Array.from(arguments);
            for (const arg of args) check(arg);
            if ((name === "repeat" || name === "padStart" || name === "padEnd") &&
                (!Number.isFinite(Number(args[0])) || Number(args[0]) > maxArray)) budget();
            // Borrowed methods can change this via call/apply. Budget the
            // actual receiver before a native method allocates its result.
            if (name === "repeat" && String(receiver).length * Number(args[0]) > maxValue) budget();
            if (typeof receiver === "string" && name === "concat" &&
                args.reduce((n, v) => n + String(v).length, receiver.length) > maxValue) budget();
            if (Array.isArray(receiver)) {
                if (receiver.length > maxArray || (name === "push" || name === "unshift") && receiver.length + args.length > maxArray) budget();
                if (name === "join" && receiver.length * String(args[0] === undefined ? "," : args[0]).length > maxValue) budget();
                // Preflight expansion before native join/toString allocates.
                if (name === "join" || name === "toString" || name === "toLocaleString") safeJSON(receiver);
            }
            return check(value.apply(receiver, args));
        };
    }
    function world(p) {
        const navigator = Object.create({userAgent: p.userAgent, language: p.language,
            languages: [p.language], hardwareConcurrency: p.concurrency, platform: p.platform});
        const location = {search: "", href: "about:blank", origin: "null", pathname: "blank"};
        const document = {scripts: [], documentElement: {getAttribute: function () { return null; }}, location: location};
        const w = {window: null, self: null, navigator: navigator, screen: {width: 0, height: 0},
            performance: {timeOrigin: p.timeOrigin, now: now}, document: document, location: location,
            localStorage: {getItem: function (k) { return k === "codex.chatgpt-conversations.device-id" ? p.deviceID : null; }},
            Math: Math, JSON: {parse: function (s) { return JSON.parse(check(String(s))); }, stringify: safeJSON},
            Object: {keys: function (o) { return check(Object.keys(o)); }, values: function (o) { return check(Object.keys(o).map(k => o[k])); },
                entries: function (o) { return check(Object.keys(o).map(k => [k, o[k]])); }, getPrototypeOf: Object.getPrototypeOf},
            Array: {isArray: Array.isArray, from: function (v) { if (v && Number(v.length) > maxArray) budget(); return check(Array.from(v)); }},
            String: String, Number: Number, Boolean: Boolean, Date: Date,
            Promise: {resolve: Promise.resolve.bind(Promise), reject: Promise.reject.bind(Promise)},
            atob: atob, btoa: btoa, crypto: {randomUUID: uuid}};
        w.window = w; w.self = w;
        return w;
    }
    function xor(text, key) {
        let out = "";
        for (let n = 0; n < text.length; n++) out += String.fromCharCode(text.charCodeAt(n) ^ key.charCodeAt(n % key.length));
        return check(out);
    }
    let depth = 0;
    function drain() {
        try {
            while (registers.get(9).length > 0) {
                if (++steps > 10000 || registers.size > 1024 || registers.get(9).length > maxArray) budget();
                const instruction = registers.get(9).shift();
                if (!Array.isArray(instruction) || instruction.length > 128) throw new TypeError("invalid instruction");
                const fn = registers.get(instruction[0]);
                const result = fn == null ? undefined : fn.apply(undefined, instruction.slice(1));
                // Source increments after executing/awaiting an instruction.
                if (result && typeof result.then === "function") return Promise.resolve(result).then(function () { count++; return drain(); });
                count++;
            }
            return Promise.resolve();
        } catch (error) { return Promise.reject(error); }
    }
    function failure(error, resolve, reject) {
        if (error && error.name === "BudgetError") reject(error);
        else resolve(btoa(count + ": " + String(error)));
    }
    function nested(destination, queue, resultRegister) {
        if (++depth > 32) budget();
        const saved = registers.get(9).slice();
        put(9, queue.slice());
        return drain().then(function () { return registers.get(resultRegister); }, function (e) {
            if (e && e.name === "BudgetError") throw e;
            if (destination !== null) put(destination, String(e));
            return String(e);
        }).finally(function () { put(9, saved); depth--; });
    }
    function schedule(dx, key, w) {
        const run = function () { return new Promise(function (resolve, reject) {
            registers = new Map(); count = 0; steps = 0; settled = false; depth = 0;
            registers.set(0, function (next) { return schedule(next, String(registers.get(16)), w); });
            registers.set(1, function (a, b) { put(a, xor(String(registers.get(a)), String(registers.get(b)))); });
            registers.set(2, put);
            registers.set(3, function (value) { if (!settled) { settled = true; resolve(btoa(String(value))); } });
            registers.set(4, function (value) { if (!settled) { settled = true; reject(btoa(String(value))); } });
            registers.set(5, function (a, b) { const v = registers.get(a); if (Array.isArray(v)) { if (v.length >= maxArray) budget(); v.push(registers.get(b)); } else put(a, v + registers.get(b)); });
            registers.set(6, function (a, b, c) { put(a, property(registers.get(b), registers.get(c), false)); });
            registers.set(7, function (a) { const args = Array.from(arguments).slice(1).map(k => registers.get(k)); return registers.get(a).apply(undefined, args); });
            registers.set(8, function (a, b) { put(a, registers.get(b)); });
            registers.set(10, w);
            registers.set(11, function (a, b) { const pattern = String(registers.get(b)); let match = null; for (const s of w.document.scripts) { const m = s.src.match(pattern); if (m && m.length) { match = m[0]; break; } } put(a, match); });
            registers.set(12, function (a) { put(a, registers); });
            registers.set(13, function (a, b) { try { registers.get(b).apply(undefined, Array.from(arguments).slice(2)); } catch (e) { if (e && e.name === "BudgetError") throw e; put(a, String(e)); } });
            registers.set(14, function (a, b) { put(a, JSON.parse(check(String(registers.get(b))))); });
            registers.set(15, function (a, b) { put(a, safeJSON(registers.get(b))); });
            registers.set(16, key);
            registers.set(17, function (a, b) {
                try {
                    const value = registers.get(b).apply(undefined, Array.from(arguments).slice(2).map(k => registers.get(k)));
                    if (value && typeof value.then === "function") return value.then(v => put(a, v)).catch(function (e) { if (e && e.name === "BudgetError") throw e; put(a, String(e)); });
                    put(a, value);
                } catch (e) { if (e && e.name === "BudgetError") throw e; put(a, String(e)); }
            });
            registers.set(18, function (a) { put(a, atob(String(registers.get(a)))); });
            registers.set(19, function (a) { put(a, btoa(String(registers.get(a)))); });
            registers.set(20, function (a, b, c) { if (registers.get(a) === registers.get(b)) return registers.get(c).apply(undefined, Array.from(arguments).slice(3)); });
            registers.set(21, function (a, b, c, d) { if (Math.abs(Number(registers.get(a)) - Number(registers.get(b))) > Number(registers.get(c))) return registers.get(d).apply(undefined, Array.from(arguments).slice(4)); });
            registers.set(22, function (a, queue) { return nested(a, queue, undefined); });
            registers.set(23, function (a, b) { if (registers.get(a) !== undefined) return registers.get(b).apply(undefined, Array.from(arguments).slice(2)); });
            registers.set(24, function (a, b, c) { const v = property(registers.get(b), registers.get(c), true); if (typeof v !== "function") throw new TypeError("not a function"); put(a, v); });
            registers.set(25, function () {}); registers.set(26, function () {}); registers.set(28, function () {});
            registers.set(27, function (a, b) { const v = registers.get(a); if (Array.isArray(v)) v.splice(v.indexOf(registers.get(b)), 1); else put(a, v - registers.get(b)); });
            registers.set(29, function (a, b, c) { put(a, Number(registers.get(b)) < Number(registers.get(c))); });
            registers.set(30, function (a, result, params, queue) {
                const hasParams = Array.isArray(queue), parameterRegisters = hasParams ? params : [], instructions = (hasParams ? queue : params) || [];
                put(a, function () {
                    if (settled) return;
                    if (hasParams) for (let n = 0; n < parameterRegisters.length; n++) put(parameterRegisters[n], arguments[n]);
                    return nested(null, instructions, result);
                });
            });
            registers.set(33, function (a, b, c) { put(a, Number(registers.get(b)) * Number(registers.get(c))); });
            registers.set(34, function (a, b) { return Promise.resolve(registers.get(b)).then(v => put(a, v)); });
            registers.set(35, function (a, b, c) { const divisor = Number(registers.get(c)); put(a, divisor === 0 ? 0 : Number(registers.get(b)) / divisor); });
            expire = function () { settled = true; resolve(String(count)); };
            try {
                put(9, JSON.parse(xor(atob(dx), key)));
                if (!Array.isArray(registers.get(9))) throw new TypeError("invalid queue");
                drain().catch(e => failure(e, resolve, reject));
            } catch (e) { failure(e, resolve, reject); }
        }); };
        const result = chain.then(run, run);
        chain = result.then(function () {}, function () {});
        return result;
    }
    return {run: function (profile, dx, key) { return schedule(dx, key, world(JSON.parse(profile))); }, expire: function () { if (expire) expire(); }};
})(__now, __uuid, __btoa, __atob)
