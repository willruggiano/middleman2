import { afterEach, describe, expect, it, vi } from "vitest";
import { createSyncStore } from "./sync.svelte.js";
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

describe("triggerSyncForRepo", () => {
  it("POSTs to /repos/{owner}/{name}/sync with the right path params", async () => {
    const client = makeStubClient();
    const store = createSyncStore({ client });

    await store.triggerSyncForRepo("acme", "widget");

    expect(client.POST).toHaveBeenCalledWith(
      "/repos/{owner}/{name}/sync",
      expect.objectContaining({
        params: { path: { owner: "acme", name: "widget" } },
      }),
    );
  });

  it("sets last_error on API failure", async () => {
    const client = makeStubClient({
      postResponse: { data: undefined, error: { detail: "boom" } },
    });
    const store = createSyncStore({ client });

    await expect(
      store.triggerSyncForRepo("acme", "widget"),
    ).rejects.toThrow("boom");

    expect(store.getSyncState()?.last_error).toBe("boom");
  });
});
