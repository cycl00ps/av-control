(() => {
  "use strict";

  let dragging = false;
  let sliderCommit = null;
  let pending = null; // { path, values, isSlider, elt }
  let seekTimer = null;
  let seekBusy = false;
  let seekDir = null;
  let presetHoldTimer = null;
  let presetHoldFired = false;
  let pendingStore = null; // preset number

  function fmtVol(v) {
    return (Math.round(Number(v) * 2) / 2).toFixed(1);
  }

  function fmtCtl(v) {
    const n = Math.round(Number(v) * 2) / 2;
    if (n > 0) return `+${n.toFixed(1)} dB`;
    if (n < 0) return `${n.toFixed(1)} dB`;
    return "0.0 dB";
  }

  function remoteEl() {
    return document.getElementById("remote");
  }

  function basePath() {
    const remote = remoteEl();
    if (!remote) return "";
    const post = remote.querySelector("[hx-post]")?.getAttribute("hx-post") || "";
    const idx = post.indexOf("/cmd/");
    return idx >= 0 ? post.slice(0, idx) : "";
  }

  function volWarn() {
    const w = Number(remoteEl()?.dataset.volWarn || 0);
    return w > 0 ? w : 0;
  }

  function currentVol() {
    return Number(remoteEl()?.dataset.vol || 0);
  }

  function crosses(target) {
    const w = volWarn();
    if (!w) return false;
    const cur = currentVol();
    return cur <= w && target > w;
  }

  function previewRange(el) {
    if (!(el instanceof HTMLInputElement) || el.type !== "range") return;
    const level = el.closest(".level");
    if (!level) return;

    const out = level.querySelector(".level__readout");
    const kind = level.getAttribute("data-level");
    let text;
    if (kind === "centre") {
      text = fmtCtl(el.value);
    } else {
      text = fmtVol(el.value);
      el.setAttribute("aria-valuetext", text);
    }
    if (out) out.textContent = text;
  }

  function showLoudDialog(target) {
    const dlg = document.getElementById("loud-dialog");
    const body = document.getElementById("loud-body");
    const w = volWarn();
    if (body) {
      body.textContent = `Volume will go to ${fmtVol(target)} (above ${fmtVol(w)}). This can be loud. Continue?`;
    }
    if (dlg) dlg.hidden = false;
  }

  function hideLoudDialog() {
    const dlg = document.getElementById("loud-dialog");
    if (dlg) dlg.hidden = true;
  }

  function cancelLoud() {
    hideLoudDialog();
    if (pending?.isSlider && pending.elt instanceof HTMLInputElement && sliderCommit) {
      pending.elt.value = sliderCommit.value;
      previewRange(pending.elt);
    }
    pending = null;
  }

  function confirmLoud() {
    hideLoudDialog();
    if (!pending) return;
    const { path, values } = pending;
    pending = null;
    const vals = Object.assign({}, values, { loud_ok: "1" });
    htmx.ajax("POST", path, {
      values: vals,
      target: "#remote",
      swap: "outerHTML",
    });
  }

  function showStoreDialog(n) {
    pendingStore = n;
    const dlg = document.getElementById("preset-store-dialog");
    const body = document.getElementById("preset-store-body");
    const freq = document.getElementById("tuner-freq")?.textContent?.trim() || "";
    const unit = document.querySelector(".tuner__unit")?.textContent?.trim() || "";
    if (body) {
      const where = freq ? ` (${freq} ${unit})` : "";
      body.textContent = `Save the current station${where} to preset ${n}?`;
    }
    if (dlg) dlg.hidden = false;
  }

  function hideStoreDialog() {
    const dlg = document.getElementById("preset-store-dialog");
    if (dlg) dlg.hidden = true;
  }

  function cancelStore() {
    hideStoreDialog();
    pendingStore = null;
  }

  function confirmStore() {
    hideStoreDialog();
    const n = pendingStore;
    pendingStore = null;
    if (!n) return;
    htmx.ajax("POST", `${basePath()}/cmd/preset_store`, {
      values: { n: String(n) },
      target: "#remote",
      swap: "outerHTML",
    });
  }

  function cmdKind(post) {
    if (!post) return null;
    if (post.endsWith("/cmd/vol_up")) return "vol_up";
    if (post.endsWith("/cmd/vol_set")) return "vol_set";
    if (post.endsWith("/cmd/defaults")) return "defaults";
    return null;
  }

  function clearSeekTimer() {
    if (seekTimer) {
      clearInterval(seekTimer);
      seekTimer = null;
    }
  }

  function stopSeek() {
    if (!seekDir && !seekTimer) return;
    clearSeekTimer();
    const wasSeeking = !!seekDir;
    seekDir = null;
    seekBusy = false;
    document.querySelectorAll(".btn--seek.is-seeking").forEach((b) => b.classList.remove("is-seeking"));
    if (wasSeeking) {
      htmx.ajax("POST", `${basePath()}/cmd/seek_stop`, {
        target: "#remote",
        swap: "outerHTML",
      });
    }
  }

  function startSeek(btn, dir, pointerId) {
    if (seekDir && seekDir !== dir) {
      stopSeek();
    }
    seekDir = dir;
    btn.classList.add("is-seeking");
    try {
      if (pointerId != null) btn.setPointerCapture(pointerId);
    } catch (_) {}
    const action = dir === "up" ? "seek_up" : "seek_down";
    htmx.ajax("POST", `${basePath()}/cmd/${action}`, {
      target: "#remote",
      swap: "none",
    });
    clearSeekTimer();
    seekTimer = setInterval(() => {
      if (seekBusy || !seekDir) return;
      seekBusy = true;
      htmx
        .ajax("POST", `${basePath()}/cmd/seek_tick`, {
          target: "#remote",
          swap: "outerHTML",
        })
        .finally(() => {
          seekBusy = false;
        });
    }, 400);
  }

  document.addEventListener(
    "pointerdown",
    (e) => {
      const el = e.target;
      if (el instanceof HTMLInputElement && el.type === "range") {
        dragging = true;
        if (el.closest('[data-level="volume"]')) {
          sliderCommit = { value: el.value };
        }
        return;
      }

      const seekBtn = el instanceof Element ? el.closest("[data-seek]") : null;
      if (seekBtn) {
        e.preventDefault();
        startSeek(seekBtn, seekBtn.getAttribute("data-seek"), e.pointerId);
        return;
      }

      const presetBtn = el instanceof Element ? el.closest(".preset-btn[data-preset]") : null;
      if (presetBtn) {
        presetHoldFired = false;
        const n = Number(presetBtn.getAttribute("data-preset"));
        clearTimeout(presetHoldTimer);
        presetHoldTimer = setTimeout(() => {
          presetHoldFired = true;
          showStoreDialog(n);
        }, 550);
      }
    },
    true
  );

  const endDrag = () => {
    dragging = false;
    stopSeek();
    clearTimeout(presetHoldTimer);
    presetHoldTimer = null;
  };
  document.addEventListener("pointerup", endDrag, true);
  document.addEventListener("pointercancel", endDrag, true);
  document.addEventListener("touchend", endDrag, true);

  document.body.addEventListener("htmx:beforeRequest", (e) => {
    const elt = e.detail.elt;
    if (!elt) return;

    // Long-press store: cancel the normal preset recall.
    if (presetHoldFired && elt.classList?.contains("preset-btn")) {
      e.preventDefault();
      presetHoldFired = false;
      return;
    }

    const post = elt.getAttribute("hx-post") || "";
    const kind = cmdKind(post);
    if (!kind) return;

    let target;
    const values = {};
    if (kind === "vol_up") {
      target = currentVol() + 0.5;
    } else if (kind === "vol_set") {
      target = Number(elt.value);
      values.value = elt.value;
    } else if (kind === "defaults") {
      target = Number(remoteEl()?.dataset.volDefault || 0);
    }

    if (!crosses(target)) return;

    e.preventDefault();
    pending = {
      path: post,
      values,
      isSlider: kind === "vol_set",
      elt,
    };
    showLoudDialog(target);
  });

  document.addEventListener("input", (e) => {
    previewRange(e.target);
  });

  let netBusy = false;

  function isNetCmdURL(url) {
    if (!url) return false;
    return /\/cmd\/(nsv|nls_select|net_back|net_down|ntc|pin_save|pin_play|pin_remove)\b/.test(url);
  }

  // Don't let the 5s poll wipe the UI mid-drag, mid-seek, or mid-NET command.
  document.body.addEventListener("htmx:beforeSwap", (e) => {
    const xhr = e.detail?.xhr;
    const url = xhr?.responseURL || "";
    const target = e.detail?.target;
    const isPoll = url.includes("/partials/remote");
    if (isPoll && (dragging || seekTimer || netBusy)) {
      if (target && (target.id === "remote" || target.querySelector?.("#remote"))) {
        e.detail.shouldSwap = false;
      }
    }
  });

  document.body.addEventListener("htmx:beforeRequest", (e) => {
    const path = e.detail?.pathInfo?.requestPath || e.detail?.elt?.getAttribute?.("hx-post") || "";
    if (isNetCmdURL(path)) {
      netBusy = true;
      remoteEl()?.classList.add("is-net-busy");
    }
  });

  document.body.addEventListener("htmx:afterRequest", (e) => {
    const path = e.detail?.pathInfo?.requestPath || e.detail?.elt?.getAttribute?.("hx-post") || "";
    if (!isNetCmdURL(path)) return;
    // Keep busy through a successful swap; clear immediately on failure.
    if (e.detail?.failed || e.detail?.xhr?.status >= 400) {
      netBusy = false;
      remoteEl()?.classList.remove("is-net-busy");
    }
  });

  document.body.addEventListener("htmx:afterSwap", () => {
    if (netBusy) {
      netBusy = false;
      remoteEl()?.classList.remove("is-net-busy");
    }
    if (seekDir) {
      const btn = document.querySelector(`[data-seek="${seekDir}"]`);
      if (btn) btn.classList.add("is-seeking");
    }
    applyNetFilter();
  });

  document.getElementById("loud-cancel")?.addEventListener("click", cancelLoud);
  document.getElementById("loud-cancel-backdrop")?.addEventListener("click", cancelLoud);
  document.getElementById("loud-confirm")?.addEventListener("click", confirmLoud);
  document.getElementById("preset-store-cancel")?.addEventListener("click", cancelStore);
  document.getElementById("preset-store-backdrop")?.addEventListener("click", cancelStore);
  document.getElementById("preset-store-confirm")?.addEventListener("click", confirmStore);
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    const loud = document.getElementById("loud-dialog");
    if (loud && !loud.hidden) cancelLoud();
    const store = document.getElementById("preset-store-dialog");
    if (store && !store.hidden) cancelStore();
  });

  function applyNetFilter() {
    const input = document.querySelector("[data-net-filter]");
    const list = document.querySelector("[data-net-list]");
    const empty = document.querySelector("[data-net-filter-empty]");
    const clearBtn = document.querySelector("[data-net-filter-clear]");
    if (!input || !list) return;
    const q = (input.value || "").trim().toLowerCase();
    if (clearBtn) clearBtn.hidden = !q;
    let shown = 0;
    list.querySelectorAll("[data-net-text]").forEach((btn) => {
      const text = (btn.getAttribute("data-net-text") || "").toLowerCase();
      const match = !q || text.includes(q);
      btn.hidden = !match;
      if (match) shown += 1;
    });
    if (empty) empty.hidden = !q || shown > 0;
  }

  document.addEventListener("input", (e) => {
    if (e.target instanceof Element && e.target.matches("[data-net-filter]")) {
      applyNetFilter();
    }
  });

  document.addEventListener("click", (e) => {
    const clear = e.target instanceof Element ? e.target.closest("[data-net-filter-clear]") : null;
    if (!clear) return;
    const input = document.querySelector("[data-net-filter]");
    if (input) {
      input.value = "";
      applyNetFilter();
      input.focus();
    }
  });
})();
