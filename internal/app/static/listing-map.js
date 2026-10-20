(() => {
  const section = document.querySelector("details.listing-map-section");
  const el = document.getElementById("listing-map");
  if (!section || !el || !window.L || !window.housingMap) return;
  const { json, store } = housingMap;

  const config = json("map-config", { positron: false, overlays: [] });
  const pin = json("listing-pin", null);
  if (!pin) return;

  let map;
  const show = () => {
    if (map) {
      map.invalidateSize();
      return;
    }
    map = L.map(el).setView([pin.lat, pin.lng], pin.approx ? 14 : 16);
    housingMap.layers(map, config);
    const color = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#2563eb";
    if (pin.approx) {
      L.circle([pin.lat, pin.lng], { radius: 400, color, weight: 2, dashArray: "4 4", fillOpacity: 0.1 }).addTo(map);
    }
    L.circleMarker([pin.lat, pin.lng], { radius: 8, color, weight: 3, fillOpacity: 0.6, dashArray: pin.approx ? "3 3" : null })
      .bindTooltip(pin.title)
      .addTo(map);
  };

  section.open = store.get("listingMapOpen", false) === true;
  if (section.open) show();
  section.addEventListener("toggle", () => {
    store.set("listingMapOpen", section.open);
    if (section.open) show();
  });
})();
