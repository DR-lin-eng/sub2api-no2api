import { beforeEach, describe, expect, it, vi } from "vitest";

const { get } = vi.hoisted(() => ({ get: vi.fn() }));

vi.mock("@/core/networks/client", () => ({
  apiClient: { get },
}));

import { getUsageSummary } from "@/features/admin-groups/data/datasources/adminGroupQueries";

describe("admin group usage summary datasource", () => {
  beforeEach(() => {
    get.mockReset();
  });

  it("uses the server timezone contract and preserves yesterday cost", async () => {
    const payload = [
      {
        group_id: 7,
        today_cost: 1.25,
        yesterday_cost: 2.5,
        total_cost: 9.75,
      },
    ];
    get.mockResolvedValue({ data: payload });

    await expect(getUsageSummary()).resolves.toEqual(payload);
    expect(get).toHaveBeenCalledWith("/admin/groups/usage-summary");
  });
});
