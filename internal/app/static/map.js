(() => {
  const range = document.querySelector("form.range");
  if (!range) return;
  for (const input of range.querySelectorAll("input[type=date]")) {
    input.addEventListener("change", () => { if (!input.value || input.checkValidity()) range.requestSubmit(); });
  }
  const quick = range.querySelector("select.quick");
  if (quick) quick.addEventListener("change", () => { if (quick.value) location.href = quick.value; });
})();
(() => {
  const el = document.getElementById("map");
  if (!el || !window.L || !window.housingMap) return;
  const { json, store } = housingMap;

  const config = json("map-config", { positron: false, overlays: [] });
  const pins = json("map-pins", []);
  const extents = json("map-extent", {});

  const picker = document.querySelector(".picker.collections");
  const boxes = picker ? [...picker.querySelectorAll("input[data-collection]")] : [];
  const colors = Object.fromEntries(boxes.map((b) => [b.dataset.collection, b.dataset.color]));
  const fading = document.querySelector("select.fading");
  const contrastLabel = document.querySelector("label.contrast");
  const contrast = contrastLabel && contrastLabel.querySelector("input");
  const shows = [...document.querySelectorAll("input[data-show]")];

  const hidden = store.get("mapHidden", []);
  for (const box of boxes) box.checked = !hidden.includes(box.dataset.collection);
  const mode = store.get("mapFading", null);
  if (fading && [...fading.options].some((o) => o.value === mode)) fading.value = mode;
  const level = store.get("mapContrast", null);
  if (contrast && level !== null) contrast.value = level;
  const shown = store.get("mapShow", {});
  for (const box of shows) box.checked = shown[box.dataset.show] === true;
  const enabled = () => new Set(boxes.filter((b) => b.checked).map((b) => b.dataset.collection));
  const showing = () => Object.fromEntries(shows.map((b) => [b.dataset.show, b.checked]));
  const visibleIn = (pin, c, show) => (show.ungraded || c in pin.scores) && (show.dealbreakers || !pin.dealbreakers?.[c]);

  const spreadOverlapping = () => {
    const groups = new Map();
    for (const p of pins) {
      const key = `${p.lat},${p.lng}`;
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(p);
    }
    for (const group of groups.values()) {
      if (group.length < 2) continue;
      const r = 0.0012 * Math.sqrt(group.length);
      group.forEach((p, i) => {
        const a = (2 * Math.PI * i) / group.length;
        p.lat += r * Math.sin(a);
        p.lng += (r * Math.cos(a)) / Math.cos((p.lat * Math.PI) / 180);
      });
    }
  };
  spreadOverlapping();

  const newTab = (a) => {
    a.target = "_blank";
    a.rel = "noopener";
    return a;
  };

  const popup = (v) => {
    const p = v.pin;
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
    meta.textContent = [p.matches[v.lead] || "Not graded yet", p.price, beds].filter(Boolean).join(" · ");
    const where = document.createElement("div");
    where.className = "muted";
    const retrieved = p.lastSeen && p.lastSeen !== p.firstSeen ? ` · last retrieved ${p.lastSeen}` : "";
    where.textContent = `First seen ${p.firstSeen}${retrieved} · ${p.collections.join(", ")}`;
    box.append(title, meta, where);
    const hits = p.dealbreakers?.[v.lead];
    if (hits) {
      const bad = document.createElement("div");
      bad.className = "bad";
      bad.textContent = `Dealbreakers: ${hits.join(", ")}`;
      box.append(bad);
    }
    if (p.approx) {
      const note = document.createElement("div");
      note.className = "muted";
      note.textContent = "Approximate: placed at the zip code's center";
      box.append(note);
    }
    return box;
  };

  const map = L.map(el);
  housingMap.layers(map, config);

  const UNGRADED = "#78716c";
  const views = pins.map((pin) => {
    const v = { pin, lead: null, value: 0 };
    let content, contentLead;
    const width = pin.preview ? { minWidth: 280, maxWidth: 280 } : { minWidth: 200, maxWidth: 320 };
    v.marker = L.circleMarker([pin.lat, pin.lng], { radius: 7, weight: 2, dashArray: pin.approx ? "3 3" : null })
      .bindTooltip("")
      .bindPopup(() => {
        if (contentLead !== v.lead) [content, contentLead] = [popup(v), v.lead];
        return content;
      }, width);
    return v;
  });

  const assignLeads = () => {
    const on = enabled();
    const show = showing();
    for (const v of views) v.lead = v.pin.collections.find((c) => on.has(c) && visibleIn(v.pin, c, show)) || null;
  };

  const fit = () => {
    const on = enabled();
    let points = views.filter((v) => v.lead).map((v) => [v.pin.lat, v.pin.lng]);
    if (!points.length) {
      const visible = Object.keys(extents).filter((c) => on.has(c));
      points = (visible.length ? visible : Object.keys(extents)).flatMap((c) => extents[c]);
    }
    const bounds = L.latLngBounds(points);
    if (bounds.isValid()) map.fitBounds(bounds.pad(0.1), { maxZoom: 15 });
    else map.setView([39.8, -98.6], 4);
  };

  const metrics = {
    age: (v) => Date.parse(v.pin.firstSeen),
    match: (v) => v.pin.scores[v.lead],
  };

  const apply = () => {
    const metric = fading && metrics[fading.value];
    const shown = [];
    for (const v of views) {
      if (!v.lead) {
        v.marker.remove();
        continue;
      }
      v.value = metric ? metric(v) : 0;
      shown.push(v);
    }
    const levels = [...new Set(shown.map((v) => v.value).filter(Number.isFinite))].sort((a, b) => a - b);
    const rank = new Map(levels.map((value, i) => [value, levels.length > 1 ? i / (levels.length - 1) : 1]));
    const gamma = 0.5 + 3.5 * ((contrast ? Number(contrast.value) : 50) / 100);
    const strength = (v) => {
      if (!metric) return 1;
      if (!rank.has(v.value)) return v.pin.ungraded ? 0.5 : 0.08;
      return 0.08 + 0.92 * rank.get(v.value) ** gamma;
    };
    if (contrast) contrast.disabled = !metric;
    shown.sort((a, b) => strength(a) - strength(b));
    for (const v of shown) {
      const s = strength(v);
      const graded = v.lead in v.pin.scores;
      const hollow = Boolean(v.pin.dealbreakers?.[v.lead]);
      v.marker
        .setRadius(metric ? 4 + 5 * s : 7)
        .setStyle({
          color: graded ? colors[v.lead] : UNGRADED,
          opacity: s,
          fillOpacity: hollow ? 0 : (v.pin.approx ? 0.2 : 0.6) * s,
        })
        .setTooltipContent([v.pin.price, v.pin.matches[v.lead] || "ungraded", hollow ? "dealbreaker" : null, v.pin.firstSeen].filter(Boolean).join(" · "))
        .addTo(map)
        .bringToFront();
    }
  };

  assignLeads();
  fit();
  apply();

  for (const box of boxes) {
    box.addEventListener("change", () => {
      store.set("mapHidden", boxes.filter((b) => !b.checked).map((b) => b.dataset.collection));
      assignLeads();
      fit();
      apply();
    });
  }
  if (fading) {
    fading.addEventListener("change", () => {
      store.set("mapFading", fading.value);
      apply();
    });
    fading.hidden = false;
  }
  for (const box of shows) {
    box.addEventListener("change", () => {
      store.set("mapShow", showing());
      assignLeads();
      apply();
    });
    box.closest("label").hidden = false;
  }
  if (contrast) {
    contrast.addEventListener("input", () => {
      store.set("mapContrast", contrast.value);
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
