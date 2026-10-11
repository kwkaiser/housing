(() => {
  const range = document.querySelector("form.range");
  if (range) {
    for (const input of range.querySelectorAll("input[type=date]")) {
      input.addEventListener("change", () => { if (!input.value || input.checkValidity()) range.requestSubmit(); });
    }
  }
})();
(() => {
  const el = document.getElementById("map");
  const data = document.getElementById("map-pins");
  if (!el || !data || !window.L) return;
  const pins = JSON.parse(data.textContent);
  const picker = document.querySelector(".picker.collections");
  const boxes = picker ? [...picker.querySelectorAll("input[data-collection]")] : [];
  const colors = Object.fromEntries(boxes.map((b) => [b.dataset.collection, b.dataset.color]));
  const fading = document.querySelector("select.fading");

  try {
    const hidden = JSON.parse(localStorage.getItem("mapHidden")) || [];
    for (const box of boxes) box.checked = !hidden.includes(box.dataset.collection);
    const mode = localStorage.getItem("mapFading");
    if (fading && [...fading.options].some((o) => o.value === mode)) fading.value = mode;
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
  L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
    maxZoom: 19,
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
  }).addTo(map);

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
    const bounds = L.latLngBounds((shown.length ? shown : pins).map((p) => [p.lat, p.lng]));
    map.fitBounds(bounds.pad(0.1), { maxZoom: 15 });
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
    const values = shown.map((i) => pins[i].value).filter(Number.isFinite);
    const low = Math.min(...values);
    const high = Math.max(...values);
    const strength = (p) => {
      if (!metric) return 1;
      if (!Number.isFinite(p.value)) return 0.15;
      return high === low ? 1 : 0.15 + (0.85 * (p.value - low)) / (high - low);
    };
    shown.sort((a, b) => strength(pins[a]) - strength(pins[b]));
    for (const i of shown) {
      const p = pins[i];
      const s = strength(p);
      markers[i]
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
  if (picker) {
    picker.hidden = false;
    document.addEventListener("click", (e) => { if (!picker.contains(e.target)) picker.open = false; });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") picker.open = false; });
  }
})();
