export function hasWailsBindings() {
  return Boolean(
    typeof window !== "undefined" &&
      window.go &&
      window.go.desktop &&
      window.go.desktop.API
  );
}

export function hasWailsRuntime() {
  return Boolean(
    typeof window !== "undefined" &&
      window.runtime &&
      typeof window.runtime.EventsOn === "function"
  );
}

async function waitForWailsBindings(timeoutMs = 4000) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    if (hasWailsBindings()) {
      return true;
    }
    await new Promise((resolve) => window.setTimeout(resolve, 25));
  }
  return hasWailsBindings();
}

export async function callWails(fn, fallbackValue) {
  if (typeof window === "undefined") {
    return fallbackValue;
  }

  if (!hasWailsBindings()) {
    const ready = await waitForWailsBindings();
    if (!ready) {
      const details = hasWailsRuntime()
        ? "Wails runtime detected, but desktop bindings were not ready."
        : "Wails runtime unavailable.";
      throw new Error(`${details} Please launch the app with \`wails dev\` or a packaged desktop build.`);
    }
  }

  return await fn();
}
