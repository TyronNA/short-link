// ShortLink front-end: wires the encode/decode forms to the JSON API.
// API base. Override at deploy time if the API host changes.
const API_BASE = "https://api.short-link.fun";

async function callJSON(path, body) {
  const res = await fetch(API_BASE + path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  let data = {};
  try { data = await res.json(); } catch (_) {}
  if (!res.ok) throw new Error(data.error || ("request failed (" + res.status + ")"));
  return data;
}

function show(el) { el.classList.add("show"); }
function hide(el) { el.classList.remove("show"); }

function wire(formId, btnId, inputId, resultId, linkId, copyId, errId, run) {
  const form = document.getElementById(formId);
  const btn = document.getElementById(btnId);
  const input = document.getElementById(inputId);
  const result = document.getElementById(resultId);
  const link = document.getElementById(linkId);
  const copy = document.getElementById(copyId);
  const err = document.getElementById(errId);
  if (!form) return;
  const errText = err.querySelector(".error-text");
  const copyLabel = copy.querySelector(".copy-label");

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    hide(result); hide(err); errText.textContent = "";
    btn.disabled = true;
    btn.classList.add("loading");
    try {
      const url = await run(input.value.trim());
      link.href = url;
      link.textContent = url;
      show(result);
    } catch (ex) {
      errText.textContent = ex.message;
      show(err);
    } finally {
      btn.disabled = false;
      btn.classList.remove("loading");
    }
  });

  copy.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(link.textContent);
      copy.classList.add("done");
      copyLabel.textContent = "Copied";
      setTimeout(() => {
        copy.classList.remove("done");
        copyLabel.textContent = "Copy";
      }, 1500);
    } catch (_) {}
  });
}

wire("encode-form", "encode-btn", "encode-input", "encode-result", "encode-link", "encode-copy", "encode-error",
  async (url) => (await callJSON("/encode", { url })).short_url);

wire("decode-form", "decode-btn", "decode-input", "decode-result", "decode-link", "decode-copy", "decode-error",
  async (shortURL) => (await callJSON("/decode", { short_url: shortURL })).original_url);
