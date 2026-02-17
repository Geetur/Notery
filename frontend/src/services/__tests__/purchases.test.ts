// purchases.test.ts — Tests for purchases service functions.

jest.mock("@/lib/api-client", () => ({
  apiGet: jest.fn(),
  apiPost: jest.fn(),
  apiDelete: jest.fn(),
}));

import {
  getCart,
  addToCart,
  removeFromCart,
  checkoutCart,
  purchaseNote,
  checkPurchaseStatus,
  getMyPurchases,
  getPurchaseHistory,
  getOrderStatus,
} from "@/services/purchases";
import { apiGet, apiPost, apiDelete } from "@/lib/api-client";

const mockApiGet = apiGet as jest.MockedFunction<typeof apiGet>;
const mockApiPost = apiPost as jest.MockedFunction<typeof apiPost>;
const mockApiDelete = apiDelete as jest.MockedFunction<typeof apiDelete>;

beforeEach(() => {
  jest.clearAllMocks();
});

describe("getCart", () => {
  it("calls /cart", async () => {
    mockApiGet.mockResolvedValue({ cart: ["1", "2"] });
    const result = await getCart();
    expect(mockApiGet).toHaveBeenCalledWith("/cart");
    expect(result).toEqual({ cart: ["1", "2"] });
  });
});

describe("addToCart", () => {
  it("calls POST /cart with note_id", async () => {
    mockApiPost.mockResolvedValue({ message: "added" });
    await addToCart(5);
    expect(mockApiPost).toHaveBeenCalledWith("/cart", { note_id: 5 });
  });
});

describe("removeFromCart", () => {
  it("calls DELETE /cart/:itemId", async () => {
    mockApiDelete.mockResolvedValue({ message: "removed" });
    await removeFromCart("5");
    expect(mockApiDelete).toHaveBeenCalledWith("/cart/5");
  });
});

describe("checkoutCart", () => {
  it("calls POST /checkout", async () => {
    mockApiPost.mockResolvedValue({ order_id: 1 });
    await checkoutCart();
    expect(mockApiPost).toHaveBeenCalledWith("/checkout", {});
  });

  it("passes idempotency key", async () => {
    mockApiPost.mockResolvedValue({ order_id: 1 });
    await checkoutCart("key123");
    expect(mockApiPost).toHaveBeenCalledWith("/checkout", {
      idempotency_key: "key123",
    });
  });
});

describe("purchaseNote", () => {
  it("calls POST /notes/:id/purchase", async () => {
    mockApiPost.mockResolvedValue({ purchase_id: 1 });
    await purchaseNote(3);
    expect(mockApiPost).toHaveBeenCalledWith("/notes/3/purchase");
  });
});

describe("checkPurchaseStatus", () => {
  it("calls GET /notes/:id/purchased", async () => {
    mockApiGet.mockResolvedValue({ purchased: true });
    const result = await checkPurchaseStatus(3);
    expect(mockApiGet).toHaveBeenCalledWith("/notes/3/purchased");
    expect(result).toEqual({ purchased: true });
  });
});

describe("getMyPurchases", () => {
  it("calls GET /me/purchases", async () => {
    mockApiGet.mockResolvedValue({ purchases: [] });
    await getMyPurchases();
    expect(mockApiGet).toHaveBeenCalledWith("/me/purchases");
  });
});

describe("getPurchaseHistory", () => {
  it("calls /me/purchases/history with pagination", async () => {
    mockApiGet.mockResolvedValue({ purchases: [], total: 0 });
    await getPurchaseHistory({ page: 1, limit: 20 });
    expect(mockApiGet).toHaveBeenCalledWith(
      "/me/purchases/history?page=1&limit=20"
    );
  });
});

describe("getOrderStatus", () => {
  it("calls GET /orders/:orderId", async () => {
    mockApiGet.mockResolvedValue({ status: "completed" });
    await getOrderStatus(10);
    expect(mockApiGet).toHaveBeenCalledWith("/orders/10");
  });
});
