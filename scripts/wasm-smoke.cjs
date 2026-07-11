// wasm-smoke.cjs — drive the real web/app.wasm outside a browser.
//
//   node scripts/wasm-smoke.cjs                                 # static bank
//   node scripts/wasm-smoke.cjs "?corpus=ollama&model=qwen3:8b" # streaming
//
// `go build` only proves the wasm target compiles. This runs it: it boots
// app.wasm on Go's own wasm_exec.js, fakes the browser globals the module
// reads, and drives the same snapshot()/dispatch() functions the page calls.
//
// It exists because it caught a bug nothing else would have: under GOOS=js,
// net/http only routes a request through fetch() when the Transport has no
// dial hooks, so the corpus worked natively but could never have reached the
// network from the browser. See corpus/ollama_client_wasm.go.
//
// Requires node; the ollama query additionally needs a running Ollama with the
// model pulled. Exits non-zero on failure so it can gate a change.

const fs = require("fs");
const path = require("path");

const ROOT = path.resolve(__dirname, "..");
const WASM = path.join(ROOT, "web/app.wasm");
const TIMEOUT_MS = 120000;

const query = process.argv[2] || "";
const wantsOllama = query.includes("corpus=ollama");

// --- browser globals the module expects -------------------------------------

// cmd/web reads its corpus selection from window.location.search.
globalThis.location = { search: query };

// Go's net/http deliberately DISABLES the Fetch API when it detects Node
// (jsFetchDisabled, go.dev/issue/57613), falling back to an in-process fake
// network where a dial to localhost always fails with "connection refused". It
// detects Node by sniffing process.argv0 — which is read-only AND
// non-configurable, and a Proxy is forbidden from lying about such a property.
// Swapping in a clone is therefore the only way to exercise the same fetch path
// a browser takes. Harness-only; nothing in the app depends on it.
if (wantsOllama) {
  const real = process;
  const fake = {};
  for (const key in real) {
    try {
      const v = real[key];
      fake[key] = typeof v === "function" ? v.bind(real) : v;
    } catch (_) {
      /* skip properties that throw on access */
    }
  }
  fake.argv0 = "browser-sim";
  globalThis.process = fake;
}

require(path.join(ROOT, "web/wasm_exec.js"));

// --- run --------------------------------------------------------------------

const go = new Go();

WebAssembly.instantiate(fs.readFileSync(WASM), go.importObject).then((res) => {
  go.run(res.instance); // Go blocks on select{} and never resolves — do not await
  const started = Date.now();
  setTimeout(check, 1000);

  function check() {
    const state = JSON.parse(globalThis.snapshot());
    const c = state.corpus;
    const t = ((Date.now() - started) / 1000).toFixed(1);

    console.log(
      `t=${t}s  phase=${String(c.phase).padEnd(9)} source=${String(c.source).padEnd(7)} ` +
        `words=${String(c.words).padStart(3)} sentences=${String(c.sentences).padStart(3)}` +
        (c.detail ? `  detail=${c.detail}` : "")
    );

    if (!wantsOllama) {
      // The default page is a fixed bank: ready immediately, no generation.
      if (c.phase !== "ready" || c.source !== "static" || c.words === 0) {
        return fail(`expected a ready static bank, got ${JSON.stringify(c)}`);
      }
      console.log("\nOK: static bank serves immediately.");
      return done(0);
    }

    // Streaming: the drill must end up on GENERATED text, not the fallback.
    if (c.phase === "streaming" && c.words > 0 && c.sentences > 0) {
      const word = JSON.parse(globalThis.dispatch(JSON.stringify({ type: "skip" }))).drill.text;
      const sentence = JSON.parse(
        globalThis.dispatch(JSON.stringify({ type: "set_content", content: "sentences" }))
      ).drill.text;

      console.log(`\n  drill word:     ${JSON.stringify(word)}`);
      console.log(`  drill sentence: ${JSON.stringify(sentence)}`);

      if (!word || !sentence) return fail("drill served empty text");
      console.log(`\nOK: streaming from ${c.source} (${c.detail}) in ${t}s.`);
      return done(0);
    }

    if (Date.now() - started > TIMEOUT_MS) {
      return fail(`never reached streaming with both kinds (last: ${JSON.stringify(c)})`);
    }
    setTimeout(check, 1000);
  }
});

function fail(msg) {
  console.error(`\nFAIL: ${msg}`);
  done(1);
}

function done(code) {
  process.exit(code); // Go's goroutines keep the event loop alive; exit explicitly.
}
