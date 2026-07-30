// Keymap Trainer — thin vanilla-JS render layer.
//
// All trainer logic lives in the Go core, compiled to WebAssembly (app.wasm).
// This file's only job is DOM: boot the WASM runtime, call the two globals it
// exposes (snapshot() / dispatch(eventJSON)), and render whatever State JSON
// comes back. No trainer state is duplicated here except the scratchpad
// (which is intentionally local-only, per the core's design) and the current
// tab highlight, which is just an echo of state.mode.
(function () {
  "use strict";

  const stage = document.getElementById("stage");
  const tabButtons = Array.from(document.querySelectorAll(".tabbtn"));

  const ARROW_GLYPH = { left: "←", down: "↓", up: "↑", right: "→" };
  const ARROW_KEYS = { ArrowLeft: "left", ArrowDown: "down", ArrowUp: "up", ArrowRight: "right" };

  let current = null; // last rendered State, for tab highlighting etc.

  // ---- WASM boot ----

  function boot() {
    const go = new Go();
    WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject)
      .then((result) => {
        go.run(result.instance);
        wireTabs();
        render(JSON.parse(window.snapshot()));
      })
      .catch((err) => {
        stage.innerHTML =
          '<p class="boot-msg">Failed to load app.wasm: ' + escapeHTML(String(err)) +
          '. If you opened this file directly (file://), serve the directory over HTTP instead ' +
          '(e.g. <code>python3 -m http.server</code> from web/), since instantiateStreaming requires it.</p>';
      });
  }

  // send an Event to the core and render the State it returns.
  function send(ev) {
    const state = JSON.parse(window.dispatch(JSON.stringify(ev)));
    render(state);
    return state;
  }

  // ---- tabs ----

  function wireTabs() {
    tabButtons.forEach((btn) => {
      btn.addEventListener("click", () => {
        send({ type: "switch_mode", mode: btn.dataset.mode });
      });
    });
  }

  function highlightTabs(mode) {
    tabButtons.forEach((btn) => btn.classList.toggle("active", btn.dataset.mode === mode));
  }

  // ---- top-level render ----

  function render(state) {
    current = state;
    highlightTabs(state.mode);
    switch (state.mode) {
      case "mirror":
        renderMirror(state);
        break;
      case "nav":
        renderNav(state);
        break;
      case "scratch":
        renderScratch(state);
        break;
      case "reference":
        renderReference(state);
        break;
      default:
        stage.innerHTML = "";
    }
  }

  // ---- mirror mode ----

  function renderMirror(state) {
    const drill = state.drill;
    if (!drill) {
      stage.innerHTML = "";
      return;
    }
    const isSentence = state.content === "sentences";

    const textHtml = drill.chars
      .map((c) => {
        const isSpace = c.char === " ";
        const disp = isSpace ? "·" : escapeHTML(c.char);
        let color = "var(--text-muted)";
        let extra = "";
        if (c.status === "done") color = "var(--text-success)";
        else if (c.status === "current") {
          color = "var(--text-accent)";
          extra = "text-decoration:underline;";
        }
        if (isSpace) extra += "opacity:0.5;";
        return '<span style="color:' + color + ";" + extra + '">' + disp + "</span>";
      })
      .join("");

    const hint = drill.hint || {};
    let hintHtml;
    if (drill.complete) {
      hintHtml = '<p class="hint" style="color:var(--text-success);">Complete!</p>';
    } else if (!hint.mapped) {
      hintHtml = '<p class="hint" style="color:var(--text-muted);">skipping unsupported char…</p>';
    } else if (hint.isSpace) {
      hintHtml =
        '<p class="hint" style="color:var(--text-secondary);">Next: <strong>space</strong> — tap the ' +
        '<span class="keycap" style="min-width:60px">spacebar</span></p>';
    } else {
      hintHtml =
        '<p class="hint" style="color:var(--text-secondary);">Next: <strong>' +
        escapeHTML(hint.output) +
        "</strong> — press " +
        (hint.holdSpace ? "<strong>space + </strong>" : "") +
        '<span class="keycap lit">' +
        escapeHTML(hint.key) +
        "</span></p>";
    }

    const feedback = drill.feedback || "";
    const feedbackColor = feedback.indexOf("!") !== -1 ? "var(--text-success)" : "var(--text-danger)";

    stage.innerHTML =
      '<div class="chip-row">' +
      chip("words", state.content === "words", "data-content") +
      chip("sentences", state.content === "sentences", "data-content") +
      (state.content === "words"
        ? '<span class="chip-sep"></span>' +
          chip("short", state.length === "short", "data-length") +
          chip("any", state.length === "any", "data-length") +
          chip("long", state.length === "long", "data-length")
        : "") +
      '<button class="chip chip-skip" id="skip">skip →</button>' +
      "</div>" +
      '<p class="panel-note">Karabiner ON. Type ' +
      (isSentence ? "the sentence" : "the word") +
      ' left-handed; the field shows what actually arrives. <span style="color:var(--text-muted)">(· = space)</span></p>' +
      '<div class="panel">' +
      '<p class="' +
      (isSentence ? "sentence" : "prompt-word") +
      '">' +
      textHtml +
      "</p>" +
      hintHtml +
      '<input id="cap" class="capture" autocomplete="off" autocapitalize="off" spellcheck="false" placeholder="type here…" />' +
      "</div>" +
      '<div class="stats">' +
      statCard("Correct", drill.stats.hits, "var(--text-success)") +
      statCard("Misses", drill.stats.misses, "var(--text-danger)") +
      statCard("Accuracy", drill.stats.accuracy + "%") +
      statCard("WPM", drill.stats.wpm || "—") +
      "</div>" +
      '<p class="hint" style="color:' +
      (feedback ? feedbackColor : "var(--text-secondary)") +
      ';">' +
      escapeHTML(feedback) +
      "</p>";

    stage.querySelectorAll("[data-content]").forEach((btn) => {
      btn.addEventListener("click", () => send({ type: "set_content", content: btn.textContent }));
    });
    stage.querySelectorAll("[data-length]").forEach((btn) => {
      btn.addEventListener("click", () => send({ type: "set_length", length: btn.textContent }));
    });
    const skip = document.getElementById("skip");
    if (skip) skip.addEventListener("click", () => send({ type: "skip" }));

    const cap = document.getElementById("cap");
    if (cap) {
      cap.focus();
      cap.addEventListener("input", onMirrorInput);
    }
  }

  function onMirrorInput(e) {
    const el = e.target;
    const val = el.value;
    el.value = "";
    if (!val) return;
    const r = val[val.length - 1];
    send({ type: "type", rune: r, atMillis: Date.now() });
    // keep focus on the capture field after re-render.
    const cap = document.getElementById("cap");
    if (cap) cap.focus();
  }

  // ---- nav mode ----

  function renderNav(state) {
    const nav = state.nav;
    if (!nav) {
      stage.innerHTML = "";
      return;
    }

    const seqHtml = nav.sequence
      .map((dir, i) => {
        let color = "var(--text-muted)";
        if (i < nav.index) color = "var(--text-success)";
        else if (i === nav.index) color = "var(--text-accent)";
        return '<span style="color:' + color + '">' + ARROW_GLYPH[dir] + "</span>";
      })
      .join("");

    const feedbackColor = "var(--text-danger)";

    stage.innerHTML =
      '<p class="panel-note">Karabiner ON. Hold <span class="keycap">b</span> (no space), tap ' +
      '<span class="keycap">a</span> <span class="keycap">s</span> <span class="keycap">d</span> ' +
      '<span class="keycap">f</span> — widget reads the real arrows.</p>' +
      '<div class="panel navstage" id="navfocus" tabindex="0">' +
      '<p class="panel-note" style="margin:0 0 8px;">click here, then navigate</p>' +
      '<p class="navseq">' +
      seqHtml +
      "</p>" +
      "</div>" +
      '<div class="stats stats-3">' +
      statCard("Correct", nav.hits, "var(--text-success)") +
      statCard("Misses", nav.misses, "var(--text-danger)") +
      statCard("Progress", nav.index + "/" + nav.sequence.length) +
      "</div>" +
      '<p class="hint" style="color:' +
      (nav.feedback ? feedbackColor : "var(--text-secondary)") +
      ';">' +
      escapeHTML(nav.feedback || "") +
      "</p>";

    const focusEl = document.getElementById("navfocus");
    if (focusEl) focusEl.focus();
  }

  function onNavKey(e) {
    if (!current || current.mode !== "nav") return;
    const dir = ARROW_KEYS[e.key];
    if (!dir) return;
    e.preventDefault();
    send({ type: "arrow", arrow: dir });
  }

  // ---- scratchpad (local only, no core dispatch) ----

  function renderScratch(_state) {
    stage.innerHTML =
      '<p class="panel-note">Free practice with Karabiner live — mirror, arrows, delete, all as your ' +
      "config produces them.</p>" +
      '<textarea id="padta" class="capture" autocomplete="off" autocapitalize="off" spellcheck="false" ' +
      'placeholder="Hold space to mirror. Hold b (no space) + asdf for arrows. caps=return, tab=delete…"></textarea>' +
      '<p class="hint" style="color:var(--text-muted);">Real text field — what you see is your actual keymap output. ' +
      "(This mode is local to the page; nothing is sent to the core.)</p>";
    const ta = document.getElementById("padta");
    if (ta) ta.focus();
  }

  // ---- reference mode ----

  function renderReference(state) {
    const rows = (state.reference || [])
      .map(
        (row) =>
          '<tr><td><span class="keycap">' +
          escapeHTML(row.key) +
          '</span></td><td class="ref-arrow">→</td><td class="ref-out">' +
          escapeHTML(row.output) +
          "</td></tr>"
      )
      .join("");

    stage.innerHTML =
      '<div class="ref-grid">' +
      "<div>" +
      '<p class="ref-title">Left key + space produces</p>' +
      '<table class="ref-table">' +
      rows +
      "</table>" +
      "</div>" +
      "<div>" +
      '<p class="ref-title">Layer keys</p>' +
      '<table class="ref-table">' +
      "<tr><td><span class=\"keycap\">caps</span></td><td class=\"ref-arrow\">→</td><td>return</td></tr>" +
      "<tr><td><span class=\"keycap\">tab</span></td><td class=\"ref-arrow\">→</td><td>delete</td></tr>" +
      "<tr><td><span class=\"keycap\">hold b</span></td><td class=\"ref-arrow\">→</td><td>arm nav (no space)</td></tr>" +
      "<tr><td><span class=\"keycap\">a s d f</span></td><td class=\"ref-arrow\">→</td><td>← ↓ ↑ →</td></tr>" +
      "</table>" +
      '<p class="ref-note">Word mode has a larger bank with length filters. Sentence mode drills continuous ' +
      "text including spaces (shown as ·). WPM is live in both.</p>" +
      "</div>" +
      "</div>";
  }

  // ---- small helpers ----

  function chip(label, active, dataAttr) {
    return (
      '<button class="chip ' +
      (active ? "active" : "") +
      '" ' +
      dataAttr +
      '="' +
      label +
      '">' +
      label +
      "</button>"
    );
  }

  function statCard(label, value, color) {
    return (
      '<div class="statcard"><p class="statlabel">' +
      escapeHTML(label) +
      '</p><p class="statval"' +
      (color ? ' style="color:' + color + '"' : "") +
      ">" +
      escapeHTML(String(value)) +
      "</p></div>"
    );
  }

  function escapeHTML(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  document.addEventListener("keydown", onNavKey);
  boot();
})();
