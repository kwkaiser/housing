(() => {
  const range = document.querySelector("form.range");
  if (range) {
    for (const input of range.querySelectorAll("input[type=date]")) {
      input.addEventListener("change", () => { if (!input.value || input.checkValidity()) range.requestSubmit(); });
    }
    const quick = range.querySelector("select.quick");
    if (quick) quick.addEventListener("change", () => { if (quick.value) location.href = quick.value; });
  }
})();
(() => {
  const el = document.getElementById("map");
  const data = document.getElementById("map-pins");
  if (!el || !data || !window.L) return;
  const pins = JSON.parse(data.textContent);
  const extentEl = document.getElementById("map-extent");
  const extents = (extentEl && JSON.parse(extentEl.textContent)) || {};
  const picker = document.querySelector(".picker.collections");
  const boxes = picker ? [...picker.querySelectorAll("input[data-collection]")] : [];
  const colors = Object.fromEntries(boxes.map((b) => [b.dataset.collection, b.dataset.color]));
  const fading = document.querySelector("select.fading");
  const contrastLabel = document.querySelector("label.contrast");
  const contrast = contrastLabel && contrastLabel.querySelector("input");

  try {
    const hidden = JSON.parse(localStorage.getItem("mapHidden")) || [];
    for (const box of boxes) box.checked = !hidden.includes(box.dataset.collection);
    const mode = localStorage.getItem("mapFading");
    if (fading && [...fading.options].some((o) => o.value === mode)) fading.value = mode;
    const level = localStorage.getItem("mapContrast");
    if (contrast && level !== null) contrast.value = level;
  } catch {}
  const enabled = () => new Set(boxes.filter((b) => b.checked).map((b) => b.dataset.collection));

  const spread = new Map();
  for (const p of pins) {
    const key = `${p.lat},${p.lng}`;
    if (!spread.has(key)) spread.set(key, []);
    spread.get(key).push(p);
  }
  for (const group of spread.values()) {
    if (group.length < 2) continue;
    const r = 0.0012 * Math.sqrt(group.length);
    group.forEach((p, i) => {
      const a = (2 * Math.PI * i) / group.length;
      p.lat += r * Math.sin(a);
      p.lng += (r * Math.cos(a)) / Math.cos((p.lat * Math.PI) / 180);
    });
  }

  const newTab = (a) => {
    a.target = "_blank";
    a.rel = "noopener";
    return a;
  };

  const popup = (p) => {
    const box = document.createElement("div");
    if (p.preview) {
      const img = document.createElement("img");
      img.className = "preview";
      img.src = p.preview;
      img.alt = "";
      const link = newTab(document.createElement("a"));
      link.href = p.href;
      link.append(img);
      box.append(link);
    }
    const title = newTab(document.createElement("a"));
    title.href = p.href;
    title.textContent = p.title;
    const meta = document.createElement("div");
    const beds = p.beds === "-" ? null : p.beds === "studio" ? p.beds : `${p.beds} bd`;
    meta.textContent = [p.matches[p.lead], p.price, beds].filter(Boolean).join(" · ");
    const where = document.createElement("div");
    where.className = "muted";
    where.textContent = `First seen ${p.firstSeen} · ${p.collections.join(", ")}`;
    box.append(title, meta, where);
    if (p.approx) {
      const note = document.createElement("div");
      note.className = "muted";
      note.textContent = "Approximate: placed at the zip code's center";
      box.append(note);
    }
    return box;
  };

  const map = L.map(el);
  const osm = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';
  const bases = {
    ...("positron" in el.dataset && {
      Positron: L.tileLayer("/tiles/positron/{z}/{x}/{y}{r}.png", {
        maxZoom: 20,
        attribution: `${osm} &copy; <a href="https://carto.com/attributions">CARTO</a>`,
      }),
    }),
    OpenStreetMap: L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", { maxZoom: 19, attribution: osm }),
  };
  const overlays = {
    "Bike lanes": L.tileLayer("https://{s}.tile-cyclosm.openstreetmap.fr/cyclosm-lite/{z}/{x}/{y}.png", {
      maxZoom: 20,
      className: "bike-lanes",
      attribution: '<a href="https://github.com/cyclosm/cyclosm-cartocss-style/releases">CyclOSM</a>',
    }),
    "Rail transit": L.tileLayer("https://{s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png", {
      maxZoom: 19,
      attribution: '<a href="https://www.openrailwaymap.org">OpenRailwayMap</a>',
    }),
  };
  let layers = {};
  try { layers = JSON.parse(localStorage.getItem("mapLayers")) || {}; } catch {}
  (bases[layers.base] || bases.Positron || bases.OpenStreetMap).addTo(map);
  for (const name of layers.overlays || []) overlays[name]?.addTo(map);
  L.control.layers(bases, overlays).addTo(map);
  const saveLayers = () => {
    const base = Object.keys(bases).find((k) => map.hasLayer(bases[k]));
    const on = Object.keys(overlays).filter((k) => map.hasLayer(overlays[k]));
    try { localStorage.setItem("mapLayers", JSON.stringify({ base, overlays: on })); } catch {}
  };
  map.on("baselayerchange overlayadd overlayremove", saveLayers);

  const markers = pins.map((p) => {
    const m = L.circleMarker([p.lat, p.lng], { radius: 7, weight: 2, dashArray: p.approx ? "3 3" : null })
      .bindTooltip("");
    let content;
    const width = p.preview ? { minWidth: 280, maxWidth: 280 } : { minWidth: 200, maxWidth: 320 };
    return m.bindPopup(() => (content ??= popup(p)), width);
  });

  const fit = () => {
    const on = enabled();
    const shown = pins.filter((p) => p.collections.some((c) => on.has(c)));
    let points = (shown.length ? shown : pins).map((p) => [p.lat, p.lng]);
    if (!points.length) {
      const visible = Object.keys(extents).filter((c) => on.has(c));
      points = (visible.length ? visible : Object.keys(extents)).flatMap((c) => extents[c]);
    }
    const bounds = L.latLngBounds(points);
    if (bounds.isValid()) map.fitBounds(bounds.pad(0.1), { maxZoom: 15 });
    else map.setView([39.8, -98.6], 4);
  };

  const metrics = {
    age: (p) => Date.parse(p.firstSeen),
    match: (p) => p.scores[p.lead],
  };

  const apply = () => {
    const on = enabled();
    const metric = fading && metrics[fading.value];
    const shown = [];
    pins.forEach((p, i) => {
      p.lead = p.collections.find((c) => on.has(c));
      if (!p.lead) {
        markers[i].remove();
        return;
      }
      p.value = metric ? metric(p) : 0;
      shown.push(i);
    });
    const levels = [...new Set(shown.map((i) => pins[i].value).filter(Number.isFinite))].sort((a, b) => a - b);
    const rank = new Map(levels.map((v, i) => [v, levels.length > 1 ? i / (levels.length - 1) : 1]));
    const gamma = 0.5 + 3.5 * ((contrast ? Number(contrast.value) : 50) / 100);
    const strength = (p) => {
      if (!metric) return 1;
      if (!rank.has(p.value)) return 0.08;
      return 0.08 + 0.92 * rank.get(p.value) ** gamma;
    };
    if (contrast) contrast.disabled = !metric;
    shown.sort((a, b) => strength(pins[a]) - strength(pins[b]));
    for (const i of shown) {
      const p = pins[i];
      const s = strength(p);
      markers[i]
        .setRadius(metric ? 4 + 5 * s : 7)
        .setStyle({ color: colors[p.lead], opacity: s, fillOpacity: (p.approx ? 0.2 : 0.6) * s })
        .setTooltipContent(`${p.price} · ${p.matches[p.lead]} · ${p.firstSeen}`)
        .addTo(map)
        .bringToFront();
    }
  };

  fit();
  apply();

  for (const box of boxes) {
    box.addEventListener("change", () => {
      try {
        localStorage.setItem("mapHidden", JSON.stringify(boxes.filter((b) => !b.checked).map((b) => b.dataset.collection)));
      } catch {}
      fit();
      apply();
    });
  }
  if (fading) {
    fading.addEventListener("change", () => {
      try { localStorage.setItem("mapFading", fading.value); } catch {}
      apply();
    });
    fading.hidden = false;
  }
  if (contrast) {
    contrast.addEventListener("input", () => {
      try { localStorage.setItem("mapContrast", contrast.value); } catch {}
      apply();
    });
    contrastLabel.hidden = false;
  }
  if (picker) {
    picker.hidden = false;
    document.addEventListener("click", (e) => { if (!picker.contains(e.target)) picker.open = false; });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") picker.open = false; });
  }
})();
