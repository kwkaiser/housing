window.housingMap = (() => {
  const json = (id, fallback, root = document) => {
    const node = root.getElementById(id);
    return (node && JSON.parse(node.textContent)) || fallback;
  };
  const store = {
    get(key, fallback) {
      try {
        const raw = localStorage.getItem(key);
        return raw === null ? fallback : JSON.parse(raw);
      } catch {
        return fallback;
      }
    },
    set(key, value) {
      try { localStorage.setItem(key, JSON.stringify(value)); } catch {}
    },
  };

  const routeLabel = (f) => {
    const box = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = f.label;
    box.append(title);
    const every = f.headway ? `every ${Math.max(1, Math.round(f.headway / 60))} min` : null;
    const detail = [f.detail, every].filter(Boolean).join(" · ");
    if (detail) {
      const line = document.createElement("div");
      line.className = "muted";
      line.textContent = detail;
      box.append(line);
    }
    return box;
  };

  const layers = (map, config) => {
    const available = Object.fromEntries(config.overlays.map((o) => [o.name, o]));
    const osm = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';
    const bases = {
      ...(config.positron && {
        Positron: L.tileLayer("/tiles/positron/{z}/{x}/{y}{r}.png", {
          maxZoom: 20,
          attribution: `${osm} &copy; <a href="https://carto.com/attributions">CARTO</a>`,
        }),
      }),
      OpenStreetMap: L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", { maxZoom: 19, attribution: osm }),
    };

    map.createPane("routes").style.zIndex = 350;
    const shared = {
      renderer: L.canvas({ pane: "routes", tolerance: 4 }),
      tooltip: L.tooltip({ direction: "top", offset: [0, -8], className: "route-tip" }),
      label: routeLabel,
    };
    const vector = (name, options) => {
      const o = available[name];
      return o && L.vectorOverlay(`/overlays/${name}/{z}/{x}/{y}.json`, { ...shared, tileMinZoom: o.minZoom, tileMaxZoom: o.maxZoom, ...options });
    };

    const BIKE = "#009400";
    const BUS = "#a16207";
    const RAIL = "#475569";
    const busTier = (f) => {
      if (f.headway && f.headway <= 600) return { weight: 4, opacity: 0.9 };
      if (f.headway && f.headway <= 1200) return { weight: 3, opacity: 0.65 };
      return { weight: 2, opacity: 0.4 };
    };
    const transitland = '<a href="https://www.transit.land/terms">Transitland</a>';

    const bikeHover = vector("bike", {
      attribution: '<a href="https://openfreemap.org">OpenFreeMap</a> &copy; <a href="https://www.openmaptiles.org/">OpenMapTiles</a>',
      style: () => ({ color: BIKE, weight: 12, opacity: 0 }),
      highlight: () => ({ color: BIKE, weight: 6, opacity: 0.95 }),
    });
    const rail = vector("rail", {
      attribution: transitland,
      style: (f) => ({ color: f.color || RAIL, weight: 4, opacity: 0.85 }),
      highlight: (f) => ({ color: f.color || RAIL, weight: 8, opacity: 1 }),
    });
    const bus = vector("bus", {
      attribution: transitland,
      style: (f) => ({ color: BUS, ...busTier(f) }),
      highlight: (f) => ({ color: BUS, weight: busTier(f).weight + 4, opacity: 1 }),
    });

    const overlays = {
      "Bike lanes": L.layerGroup([
        L.tileLayer("https://{s}.tile-cyclosm.openstreetmap.fr/cyclosm-lite/{z}/{x}/{y}.png", {
          maxZoom: 20,
          className: "bike-lanes",
          attribution: '<a href="https://github.com/cyclosm/cyclosm-cartocss-style/releases">CyclOSM</a>',
        }),
        ...(bikeHover ? [bikeHover] : []),
      ]),
      "Rail transit":
        rail ||
        L.tileLayer("https://{s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png", {
          maxZoom: 19,
          attribution: '<a href="https://www.openrailwaymap.org">OpenRailwayMap</a>',
        }),
      ...(bus && { "Bus routes": bus }),
    };

    const saved = store.get("mapLayers", {});
    (bases[saved.base] || bases.Positron || bases.OpenStreetMap).addTo(map);
    for (const name of saved.overlays || []) overlays[name]?.addTo(map);
    L.control.layers(bases, overlays).addTo(map);
    map.on("baselayerchange overlayadd overlayremove", () => {
      store.set("mapLayers", {
        base: Object.keys(bases).find((k) => map.hasLayer(bases[k])),
        overlays: Object.keys(overlays).filter((k) => map.hasLayer(overlays[k])),
      });
    });
  };

  return { json, store, layers };
})();
