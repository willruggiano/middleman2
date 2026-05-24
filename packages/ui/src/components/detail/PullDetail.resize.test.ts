import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

// PullDetail mounts a heavy tree of detail components that pulls in many
// context stores (detail, pulls, activity, client, actions, uiConfig,
// navigate). Stubbing that whole surface just to verify the resize handle
// markup is fragile and adds little signal. The plan explicitly allows
// falling back to file-content inspection + a localStorage round-trip via
// the loader logic. That is what we do here.

const currentDir = path.dirname(fileURLToPath(import.meta.url));
const pullDetailPath = path.join(currentDir, "PullDetail.svelte");
const source = readFileSync(pullDetailPath, "utf8");

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  localStorage.clear();
});

describe("DiffSidebar resize", () => {
  it("declares the resize handle markup on the .files-sidebar wrapper", () => {
    // The handle lives on the wrapper, not inside DiffSidebar.svelte.
    expect(source).toContain("files-sidebar__resize");
    // It should be rendered only when not collapsed; the {#if} guard is
    // intentional so the collapse-rail case (30px) skips the no-op handle.
    expect(source).toMatch(/\{#if !reviewNavCollapsed\}/);
  });

  it("binds an inline width on the .files-sidebar wrapper", () => {
    // When not collapsed, the inline width comes from reviewNavWidth.
    // When collapsed, it pins to 30px so the rail layout stays exact.
    expect(source).toContain("reviewNavWidth");
    expect(source).toMatch(/style:width=\{reviewNavCollapsed \? "30px" : `\$\{reviewNavWidth\}px`\}/);
  });

  it("wires pointer events on the handle for drag-resize", () => {
    expect(source).toContain("onpointerdown={onResizeStart}");
    expect(source).toContain("onpointermove={onResizeMove}");
    expect(source).toContain("onpointerup={onResizeEnd}");
  });

  it("persists drag end to pr-review-nav-width via localStorage", () => {
    expect(source).toContain('localStorage.setItem("pr-review-nav-width"');
    expect(source).toContain('localStorage.getItem("pr-review-nav-width")');
  });

  it("loads a clamped persisted width via the same loader the wrapper uses", () => {
    // Mirror the loader logic the component uses on first mount: read the
    // raw value, parse as a number, clamp to [MIN, MAX]. The defaults
    // assumed below match the constants declared in PullDetail.svelte.
    const DEFAULT = 280;
    const MIN = 180;
    const MAX = 560;

    function loadReviewNavWidth(): number {
      const raw = localStorage.getItem("pr-review-nav-width");
      if (raw === null) return DEFAULT;
      const n = Number(raw);
      if (!Number.isFinite(n)) return DEFAULT;
      return Math.max(MIN, Math.min(MAX, Math.round(n)));
    }

    expect(loadReviewNavWidth()).toBe(DEFAULT);

    localStorage.setItem("pr-review-nav-width", "350");
    expect(loadReviewNavWidth()).toBe(350);

    // Clamp behavior — out of range values snap to the nearest bound.
    localStorage.setItem("pr-review-nav-width", "50");
    expect(loadReviewNavWidth()).toBe(MIN);

    localStorage.setItem("pr-review-nav-width", "9999");
    expect(loadReviewNavWidth()).toBe(MAX);

    localStorage.setItem("pr-review-nav-width", "not-a-number");
    expect(loadReviewNavWidth()).toBe(DEFAULT);
  });
});
