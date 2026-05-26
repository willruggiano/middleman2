import { afterEach, describe, expect, it, vi } from "vitest";
import { createDiffStore } from "./diff.svelte.js";
import type { MiddlemanClient } from "../types.js";

function makeStubClient(opts: {
  postResponse?: { data?: unknown; error?: { detail?: string } };
} = {}): MiddlemanClient {
  return {
    GET: vi.fn(async () => ({ data: undefined, error: undefined })),
    POST: vi.fn(async () =>
      opts.postResponse ?? { data: undefined, error: undefined },
    ),
    DELETE: vi.fn(async () => ({ data: undefined, error: undefined })),
  } as unknown as MiddlemanClient;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("diff store refresh", () => {
  it("POSTs to the typed PR sync endpoint with the right path params", async () => {
    const client = makeStubClient();
    const store = createDiffStore({ client });
    store.setActivePR("acme", "widget", 42);

    await store.refresh();

    expect(client.POST).toHaveBeenCalledWith(
      "/repos/{owner}/{name}/pulls/{number}/sync",
      expect.objectContaining({
        params: { path: { owner: "acme", name: "widget", number: 42 } },
      }),
    );
  });

  it("sets refreshError from the server detail on failure", async () => {
    const client = makeStubClient({
      postResponse: { data: undefined, error: { detail: "boom" } },
    });
    const store = createDiffStore({ client });
    store.setActivePR("acme", "widget", 42);

    await store.refresh();

    expect(store.getRefreshError()).toBe("boom");
  });
});
