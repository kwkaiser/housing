L.VectorOverlay = L.Layer.extend({
  options: {
    minZoom: 0,
    maxZoom: 14,
    tileSize: 256,
    maxTiles: 64,
    renderer: null,
    tooltip: null,
    label: (f) => f.label,
    style: () => ({}),
    highlight: () => ({}),
    attribution: "",
  },

  initialize(url, options) {
    L.setOptions(this, options);
    this._url = url;
    this._tiles = new Map();
    this._byId = new Map();
    this._hovered = null;
  },

  onAdd(map) {
    map.on("moveend", this._load, this);
    map.whenReady(this._load, this);
  },

  onRemove(map) {
    map.off("moveend", this._load, this);
    for (const key of [...this._tiles.keys()]) this._drop(key);
    if (this._hovered !== null && this.options.tooltip) map.closeTooltip(this.options.tooltip);
    this._hovered = null;
  },

  getAttribution() {
    return this.options.attribution;
  },

  _wanted() {
    const map = this._map;
    const { minZoom, maxZoom, tileSize, maxTiles } = this.options;
    const zoom = Math.round(map.getZoom());
    const want = new Map();
    if (zoom < minZoom) return want;
    const z = Math.min(zoom, maxZoom);
    const bounds = map.getBounds();
    const nw = map.project(bounds.getNorthWest(), z).divideBy(tileSize).floor();
    const se = map.project(bounds.getSouthEast(), z).divideBy(tileSize).floor();
    const last = 2 ** z - 1;
    for (let x = Math.max(0, nw.x); x <= Math.min(last, se.x); x++) {
      for (let y = Math.max(0, nw.y); y <= Math.min(last, se.y); y++) want.set(`${z}/${x}/${y}`, { z, x, y });
    }
    return want.size > maxTiles ? new Map() : want;
  },

  _load() {
    if (!this._map) return;
    const want = this._wanted();
    for (const key of [...this._tiles.keys()]) if (!want.has(key)) this._drop(key);
    for (const [key, coords] of want) if (!this._tiles.has(key)) this._fetch(key, coords);
  },

  _fetch(key, coords) {
    const entry = { ctrl: new AbortController(), lines: [] };
    this._tiles.set(key, entry);
    fetch(L.Util.template(this._url, coords), { signal: entry.ctrl.signal })
      .then((r) => (r.ok ? r.json() : { features: [] }))
      .then(({ features }) => {
        if (this._tiles.get(key) !== entry) return;
        for (const f of features) this._add(entry, f);
      })
      .catch(() => {});
  },

  _add(entry, f) {
    const { renderer, tooltip, label, style, highlight } = this.options;
    const line = L.polyline(f.lines, { renderer, ...(this._hovered === f.id ? highlight(f) : style(f)) });
    line.feature = f;
    line.on("mouseover", (e) => {
      this._hover(f.id);
      if (tooltip) this._map.openTooltip(tooltip.setContent(label(f)), e.latlng);
    });
    line.on("mousemove", (e) => tooltip && tooltip.setLatLng(e.latlng));
    line.on("mouseout", () => {
      this._hover(null);
      if (tooltip) this._map.closeTooltip(tooltip);
    });
    line.addTo(this._map);
    entry.lines.push(line);
    if (!this._byId.has(f.id)) this._byId.set(f.id, new Set());
    this._byId.get(f.id).add(line);
  },

  _hover(id) {
    const { style, highlight } = this.options;
    for (const line of this._byId.get(this._hovered) || []) line.setStyle(style(line.feature));
    this._hovered = id;
    for (const line of this._byId.get(id) || []) line.setStyle(highlight(line.feature)).bringToFront();
  },

  _drop(key) {
    const entry = this._tiles.get(key);
    entry.ctrl.abort();
    for (const line of entry.lines) {
      line.remove();
      this._byId.get(line.feature.id)?.delete(line);
    }
    this._tiles.delete(key);
  },
});

L.vectorOverlay = (url, options) => new L.VectorOverlay(url, options);
