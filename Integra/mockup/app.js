(() => {
  "use strict";

  const INPUTS = [
    { id: "10", name: "XBOX" },
    { id: "02", name: "GAME" },
    { id: "01", name: "PS5" },
    { id: "11", name: "Shield" },
    { id: "05", name: "PC" },
    { id: "03", name: "AUX" },
    { id: "23", name: "CD" },
    { id: "12", name: "TV" },
    { id: "22", name: "PHONO" },
    { id: "25", name: "AM" },
    { id: "24", name: "FM" },
    { id: "2B", name: "NET" },
    { id: "2E", name: "BT" },
  ];

  const LMD = {
    "40": "Straight Decode",
    "82": "Neural:X",
  };

  const DSS = {
    "00": "Dirac Off",
    "01": "Dirac Slot 1",
    "02": "Dirac Slot 2",
    "03": "Dirac Slot 3",
  };

  // Fake presets: FM MHz / AM kHz
  const FM_PRESETS = [88.5, 90.9, 92.1, 94.5, 96.9, 99.7, 101.7, 105.3];
  const AM_PRESETS = [540, 630, 720, 810, 900, 1010, 1170, 1440];

  const TPD_RAW = "F 82C 28";

  const DEFAULTS = {
    power: true,
    mute: false,
    volDisplay: 40,
    inputId: "11",
    lmd: "40",
    ctl: 10,
    ltn: "01",
    dss: "00",
    eqs: "00",
    // Tuner (ISCP-shaped): FM wire 09210 → 92.10 MHz; AM as kHz
    fmHz: 92.1,
    amKhz: 810,
    preset: null,
  };

  const state = { ...DEFAULTS };

  const $ = (id) => document.getElementById(id);

  function fmtVol(v) {
    return (Math.round(v * 2) / 2).toFixed(1);
  }

  function fmtCtl(n) {
    if (n > 0) return `+${n} dB`;
    if (n < 0) return `${n} dB`;
    return "0 dB";
  }

  function parseTpdCelsius(raw) {
    const m = String(raw).match(/(\d+)\s*C/i);
    return m ? Number(m[1]) : null;
  }

  function inputName() {
    return INPUTS.find((i) => i.id === state.inputId)?.name ?? state.inputId;
  }

  function isTuner() {
    return state.inputId === "24" || state.inputId === "25";
  }

  function isFm() {
    return state.inputId === "24";
  }

  function fmtFreq() {
    if (isFm()) return state.fmHz.toFixed(2);
    return String(state.amKhz);
  }

  function fldLine() {
    if (isTuner()) {
      const band = isFm() ? "FM" : "AM";
      const freq = isFm() ? `${state.fmHz.toFixed(2)} MHz` : `${state.amKhz} kHz`;
      return `${band.padEnd(4, " ")}${freq}`;
    }
    const name = inputName().padEnd(10, " ");
    return `${name}${fmtVol(state.volDisplay)}`;
  }

  function clampVol(v) {
    return Math.min(100, Math.max(0, Math.round(v * 2) / 2));
  }

  function clampCtl(n) {
    return Math.min(12, Math.max(-12, n | 0));
  }

  function clampFm(hz) {
    // NRI: 87500–108000 step 50 Hz → 87.50–108.00 step 0.05
    const stepped = Math.round(hz * 20) / 20;
    return Math.min(108.0, Math.max(87.5, stepped));
  }

  function clampAm(khz) {
    // NRI: 522–1611 step 9
    const base = 522;
    const stepped = base + Math.round((khz - base) / 9) * 9;
    return Math.min(1611, Math.max(522, stepped));
  }

  function openStandbyDialog() {
    const dlg = $("standby-dialog");
    dlg.hidden = false;
    $("standby-confirm").focus();
  }

  function closeStandbyDialog() {
    $("standby-dialog").hidden = true;
    $("btn-power").focus();
  }

  function render() {
    const on = state.power;
    document.body.classList.toggle("is-standby", !on);

    const powerEl = $("st-power");
    powerEl.textContent = on ? "On" : "Standby";
    powerEl.dataset.on = on ? "true" : "false";

    $("st-input").textContent = inputName();
    $("st-vol").textContent = fmtVol(state.volDisplay);

    const muteEl = $("st-mute");
    if (state.mute) muteEl.removeAttribute("hidden");
    else muteEl.setAttribute("hidden", "");

    $("st-lmd").textContent = LMD[state.lmd] ?? state.lmd;
    $("st-dirac").textContent = DSS[state.dss] ?? state.dss;
    $("st-fld").textContent = fldLine();

    $("vol-slider").value = String(state.volDisplay);
    $("vol-readout").textContent = fmtVol(state.volDisplay);
    $("vol-slider").setAttribute("aria-valuetext", fmtVol(state.volDisplay));

    const muteBtn = $("btn-mute");
    muteBtn.textContent = state.mute ? "Unmute" : "Mute";
    muteBtn.classList.toggle("btn--mute-on", state.mute);

    $("btn-power").textContent = on ? "Standby" : "Power on";

    document.querySelectorAll("#input-grid .btn").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.input === state.inputId);
    });

    document.querySelectorAll("#lmd-group .seg").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.lmd === state.lmd);
    });
    document.querySelectorAll("#ltn-group .seg").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.ltn === state.ltn);
    });
    document.querySelectorAll("#dss-group .seg").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.dss === state.dss);
    });
    document.querySelectorAll("#eqs-group .seg").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.eqs === state.eqs);
    });

    $("ctl-slider").value = String(state.ctl);
    $("ctl-readout").textContent = fmtCtl(state.ctl);

    // Temp °C from TPD-shaped string
    const celsius = parseTpdCelsius(TPD_RAW);
    $("det-tpd").textContent = celsius != null ? `${celsius} °C` : "—";
    $("det-tpd-raw").textContent = `TPD ${TPD_RAW}`;

    if (state.lmd === "82") {
      $("det-ifa").textContent = "HDMI 4 · DTS-HD · 48 kHz · 5.1 · Neural:X → 3.1";
    } else {
      $("det-ifa").textContent = "HDMI 4 · DTS-HD · 48 kHz · 5.1 → 3.1 (Straight)";
    }

    // Tuner panel
    const tunerSec = $("tuner-section");
    if (isTuner()) {
      tunerSec.hidden = false;
      document.querySelectorAll("#band-group .seg").forEach((btn) => {
        btn.classList.toggle("is-active", btn.dataset.band === state.inputId);
      });
      $("tune-freq").textContent = fmtFreq();
      $("tune-unit").textContent = isFm() ? "MHz" : "kHz";
      $("tune-preset-label").textContent =
        state.preset != null ? `Preset ${state.preset}` : "Preset —";

      const presets = isFm() ? FM_PRESETS : AM_PRESETS;
      const cur = isFm() ? state.fmHz : state.amKhz;
      document.querySelectorAll("#preset-grid .btn").forEach((btn, i) => {
        const val = presets[i];
        const active =
          state.preset === i + 1 ||
          (isFm() ? Math.abs(val - cur) < 0.001 : val === cur);
        btn.classList.toggle("is-active", active && state.preset === i + 1);
        btn.textContent = String(i + 1);
      });
    } else {
      tunerSec.hidden = true;
    }
  }

  function buildInputs() {
    const grid = $("input-grid");
    grid.replaceChildren();
    for (const inp of INPUTS) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "btn";
      btn.dataset.input = inp.id;
      btn.textContent = inp.name;
      btn.addEventListener("click", () => {
        state.inputId = inp.id;
        state.preset = null;
        render();
      });
      grid.appendChild(btn);
    }
  }

  function buildPresets() {
    const grid = $("preset-grid");
    grid.replaceChildren();
    for (let i = 1; i <= 8; i++) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "btn";
      btn.dataset.preset = String(i);
      btn.textContent = String(i);
      btn.addEventListener("click", () => {
        const idx = i - 1;
        state.preset = i;
        if (isFm()) state.fmHz = clampFm(FM_PRESETS[idx]);
        else state.amKhz = clampAm(AM_PRESETS[idx]);
        render();
      });
      grid.appendChild(btn);
    }
  }

  function wire() {
    $("vol-down").addEventListener("click", () => {
      state.volDisplay = clampVol(state.volDisplay - 1);
      render();
    });
    $("vol-up").addEventListener("click", () => {
      state.volDisplay = clampVol(state.volDisplay + 1);
      render();
    });
    $("vol-slider").addEventListener("input", (e) => {
      state.volDisplay = clampVol(Number(e.target.value));
      render();
    });

    $("btn-mute").addEventListener("click", () => {
      state.mute = !state.mute;
      render();
    });

    $("btn-power").addEventListener("click", () => {
      if (state.power) {
        openStandbyDialog();
        return;
      }
      state.power = true;
      render();
    });

    $("standby-cancel").addEventListener("click", closeStandbyDialog);
    $("standby-backdrop").addEventListener("click", closeStandbyDialog);
    $("standby-confirm").addEventListener("click", () => {
      state.power = false;
      closeStandbyDialog();
      render();
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && !$("standby-dialog").hidden) {
        closeStandbyDialog();
      }
    });

    $("btn-defaults").addEventListener("click", () => {
      Object.assign(state, {
        ...DEFAULTS,
        power: state.power,
        fmHz: DEFAULTS.fmHz,
        amKhz: DEFAULTS.amKhz,
        preset: null,
      });
      render();
    });

    $("ctl-down").addEventListener("click", () => {
      state.ctl = clampCtl(state.ctl - 1);
      render();
    });
    $("ctl-up").addEventListener("click", () => {
      state.ctl = clampCtl(state.ctl + 1);
      render();
    });
    $("ctl-slider").addEventListener("input", (e) => {
      state.ctl = clampCtl(Number(e.target.value));
      render();
    });

    $("lmd-group").addEventListener("click", (e) => {
      const btn = e.target.closest("[data-lmd]");
      if (!btn) return;
      state.lmd = btn.dataset.lmd;
      render();
    });
    $("ltn-group").addEventListener("click", (e) => {
      const btn = e.target.closest("[data-ltn]");
      if (!btn) return;
      state.ltn = btn.dataset.ltn;
      render();
    });
    $("dss-group").addEventListener("click", (e) => {
      const btn = e.target.closest("[data-dss]");
      if (!btn) return;
      state.dss = btn.dataset.dss;
      render();
    });
    $("eqs-group").addEventListener("click", (e) => {
      const btn = e.target.closest("[data-eqs]");
      if (!btn) return;
      state.eqs = btn.dataset.eqs;
      render();
    });

    $("band-group").addEventListener("click", (e) => {
      const btn = e.target.closest("[data-band]");
      if (!btn) return;
      state.inputId = btn.dataset.band;
      state.preset = null;
      render();
    });

    $("tune-down").addEventListener("click", () => {
      state.preset = null;
      if (isFm()) state.fmHz = clampFm(state.fmHz - 0.05);
      else state.amKhz = clampAm(state.amKhz - 9);
      render();
    });
    $("tune-up").addEventListener("click", () => {
      state.preset = null;
      if (isFm()) state.fmHz = clampFm(state.fmHz + 0.05);
      else state.amKhz = clampAm(state.amKhz + 9);
      render();
    });
  }

  buildInputs();
  buildPresets();
  wire();
  render();
})();
