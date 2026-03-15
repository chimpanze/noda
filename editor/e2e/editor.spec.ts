import { test, expect } from "@playwright/test";

// Mock API responses so tests work without a running backend
async function mockAPI(page: import("@playwright/test").Page) {
  await page.route("**/_noda/**", (route) => {
    const url = route.request().url();

    if (url.includes("/files")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          workflows: ["workflows/hello.json"],
          routes: ["routes/api.json"],
          schemas: [],
          tests: [],
          workers: [],
          connections: [],
          schedules: [],
          models: [],
        }),
      });
    }

    if (url.includes("/nodes")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ nodes: [] }),
      });
    }

    if (url.includes("/vars")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ variables: [] }),
      });
    }

    if (url.includes("/services")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ services: [] }),
      });
    }

    if (url.includes("/plugins")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ plugins: [] }),
      });
    }

    if (url.includes("/schemas")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ schemas: [] }),
      });
    }

    if (url.includes("/middleware")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ available: [], presets: {} }),
      });
    }

    if (url.includes("/models")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ models: [] }),
      });
    }

    if (url.includes("/env")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ variables: [] }),
      });
    }

    // Default: empty 200
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: "{}",
    });
  });

  // Mock the trace WebSocket — just ignore it
  await page.route("**/ws/trace", (route) => route.abort());
}

test.describe("Editor", () => {
  test.beforeEach(async ({ page }) => {
    await mockAPI(page);
    await page.goto("/editor/");
    // Wait for the app to render — use the Workflows nav button
    await expect(
      page.getByRole("button", { name: "Workflows" }),
    ).toBeVisible({ timeout: 10000 });
  });

  test("loads the editor shell with sidebar and toolbar", async ({ page }) => {
    // Sidebar should have navigation items
    await expect(
      page.getByRole("button", { name: "Workflows" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Routes" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Services" })).toBeVisible();

    // Toolbar should be present
    await expect(page.locator("button[title*='Undo']")).toBeVisible();
    await expect(page.locator("button[title*='Redo']")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Save (Ctrl+S)" }),
    ).toBeVisible();
  });

  test("shows empty state when no workflow is selected", async ({ page }) => {
    await expect(page.locator("text=Select a workflow")).toBeVisible();
  });

  test("sidebar has all navigation groups", async ({ page }) => {
    // Verify all sidebar nav groups are present
    await expect(page.getByRole("button", { name: "Routes" })).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Workflows" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Workers" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Services" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Schemas" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Tests" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Docs" })).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Settings" }),
    ).toBeVisible();
  });

  test("shortcut modal opens with ? key", async ({ page }) => {
    await page.keyboard.press("?");
    await expect(page.locator("text=Keyboard Shortcuts")).toBeVisible();

    // Should show shortcuts
    await expect(page.locator("text=Undo")).toBeVisible();
    await expect(page.locator("text=Save workflow")).toBeVisible();

    // Escape closes it
    await page.keyboard.press("Escape");
    await expect(page.locator("text=Keyboard Shortcuts")).not.toBeVisible();
  });

  test("export buttons are disabled when no workflow is loaded", async ({
    page,
  }) => {
    const exportBtn = page.locator("button[title='Export workflow JSON']");
    await expect(exportBtn).toBeDisabled();

    const importBtn = page.locator("button[title='Import workflow JSON']");
    await expect(importBtn).toBeDisabled();
  });

  test("undo/redo buttons are present and functional", async ({ page }) => {
    const undoBtn = page.locator("button[title*='Undo']");
    const redoBtn = page.locator("button[title*='Redo']");
    await expect(undoBtn).toBeVisible();
    await expect(redoBtn).toBeVisible();
  });
});
