(() => {
  const el = document.getElementById("map");
  const data = document.getElementById("map-pins");
  if (!el || !data || !window.L) return;
  const pins = JSON.parse(data.textContent);
  const accent = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#2563eb";

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

  const popup = (p) => {
    const box = document.createElement("div");
    const link = document.createElement("a");
    link.href = p.href;
    link.textContent = p.title;
    const meta = document.createElement("div");
    const beds = p.beds === "-" ? null : p.beds === "studio" ? p.beds : `${p.beds} bd`;
    meta.textContent = [`#${p.rank}`, p.match, p.price, beds].filter(Boolean).join(" · ");
    box.append(link, meta);
    if (p.approx) {
      const note = document.createElement("div");
      note.className = "muted";
      note.textContent = "Approximate: placed at the zip code's center";
      box.append(note);
    }
    return box;
  };

  let map;
  const init = () => {
    if (map) {
      map.invalidateSize();
      return;
    }
    map = L.map(el, { scrollWheelZoom: false });
    map.on("focus", () => map.scrollWheelZoom.enable());
    map.on("blur", () => map.scrollWheelZoom.disable());
    L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
      maxZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    }).addTo(map);
    const markers = pins.map((p) =>
      L.circleMarker([p.lat, p.lng], {
        radius: 7,
        color: accent,
        weight: 2,
        fillOpacity: p.approx ? 0.15 : 0.6,
        dashArray: p.approx ? "3 3" : null,
      })
        .bindTooltip(`#${p.rank} · ${p.price}`)
        .bindPopup(popup(p))
        .addTo(map),
    );
    map.fitBounds(L.featureGroup(markers).getBounds().pad(0.1), { maxZoom: 15 });
  };

  const section = el.closest("details");
  if (section) {
    try {
      if (localStorage.getItem("map") === "closed") section.open = false;
    } catch {}
    section.addEventListener("toggle", () => {
      try { localStorage.setItem("map", section.open ? "open" : "closed"); } catch {}
      if (section.open) init();
    });
  }
  if (!section || section.open) init();
})();
