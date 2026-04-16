(function () {
  const eventStore = (window.blackchainEvents = window.blackchainEvents || []);

  function track(name, detail) {
    eventStore.push({
      name: name,
      detail: detail || {},
      at: new Date().toISOString(),
    });
  }

  const page = document.body.dataset.page;
  if (page) {
    track(page + "_view");
  }

  document.querySelectorAll("[data-track-event]").forEach(function (node) {
    node.addEventListener("click", function () {
      track(node.dataset.trackEvent, {
        label: node.dataset.trackLabel || node.textContent.trim(),
        href: node.getAttribute("href") || "",
      });
    });
  });

  const form = document.querySelector("[data-territory-form]");
  if (!form) {
    return;
  }

  track("territory_form_view");

  let started = false;
  const success = document.querySelector("[data-form-success]");
  const error = document.querySelector("[data-form-error]");

  function setMessage(node, text) {
    node.textContent = text;
    node.classList.add("is-visible");
  }

  function clearMessages() {
    [success, error].forEach(function (node) {
      node.textContent = "";
      node.classList.remove("is-visible");
    });
  }

  form.addEventListener("input", function () {
    if (!started) {
      started = true;
      track("territory_form_start");
    }
  });

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    clearMessages();

    if (!form.reportValidity()) {
      track("territory_form_submit_error", { reason: "validation" });
      setMessage(error, "Please complete the required fields before submitting.");
      return;
    }

    track("territory_form_submit");
    track("territory_form_submit_success");

    const payload = Object.fromEntries(new FormData(form).entries());
    window.blackchainLastTerritorySubmission = payload;

    form.reset();
    started = false;
    setMessage(success, "Territory interest submitted. The BlackChain team can follow up on launch fit, operator path, and activation readiness.");
    window.location.hash = "submitted";
  });
})();
