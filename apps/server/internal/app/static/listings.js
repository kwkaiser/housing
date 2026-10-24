(() => {
  const collection = document.querySelector("select.collection");
  if (collection) collection.addEventListener("change", () => { location.href = collection.value; });
})();
(() => {
  const table = document.querySelector("table.listings");
  const picker = document.querySelector(".picker");
  if (!table || !picker) return;
  let shown = {};
  try { shown = JSON.parse(localStorage.getItem("columns")) || {}; } catch {}
  const apply = (key, on) => {
    for (const cell of table.querySelectorAll(`[data-col="${CSS.escape(key)}"]`)) cell.classList.toggle("off", !on);
  };
  for (const box of picker.querySelectorAll("input[data-col]")) {
    const key = box.dataset.col;
    if (typeof shown[key] === "boolean") box.checked = shown[key];
    apply(key, box.checked);
    box.addEventListener("change", () => {
      shown[key] = box.checked;
      try { localStorage.setItem("columns", JSON.stringify(shown)); } catch {}
      apply(key, box.checked);
    });
  }
  picker.hidden = false;
  document.addEventListener("click", (e) => { if (!picker.contains(e.target)) picker.open = false; });
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") picker.open = false; });
  const day = document.querySelector("form.day input[name=day]");
  if (day) day.addEventListener("change", () => day.form.requestSubmit());
})();
