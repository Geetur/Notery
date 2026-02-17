// feed-store.test.ts — Tests for the Zustand feed preferences store.
import { useFeedStore } from "@/stores/feed-store";

describe("useFeedStore", () => {
  beforeEach(() => {
    useFeedStore.setState({
      sort: "hot",
      timeFilter: "day",
      viewMode: "card",
    });
  });

  it("has correct initial state", () => {
    const state = useFeedStore.getState();
    expect(state.sort).toBe("hot");
    expect(state.timeFilter).toBe("day");
    expect(state.viewMode).toBe("card");
  });

  it("setSort updates sort preference", () => {
    useFeedStore.getState().setSort("new");
    expect(useFeedStore.getState().sort).toBe("new");

    useFeedStore.getState().setSort("top");
    expect(useFeedStore.getState().sort).toBe("top");
  });

  it("setTimeFilter updates time filter", () => {
    useFeedStore.getState().setTimeFilter("week");
    expect(useFeedStore.getState().timeFilter).toBe("week");

    useFeedStore.getState().setTimeFilter("all");
    expect(useFeedStore.getState().timeFilter).toBe("all");
  });

  it("setViewMode toggles between card and compact", () => {
    useFeedStore.getState().setViewMode("compact");
    expect(useFeedStore.getState().viewMode).toBe("compact");

    useFeedStore.getState().setViewMode("card");
    expect(useFeedStore.getState().viewMode).toBe("card");
  });
});
