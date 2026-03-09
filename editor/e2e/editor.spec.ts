import { test, expect } from "@playwright/test";

test.describe("Editor", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    // Wait for the app to load
    await expect(page.locator("[data-testid='sidebar']").or(page.locator("text=Workflows"))).toBeVisible({
      timeout: 10000,
    });
  });

  test("loads the editor shell with sidebar and toolbar", async ({ page }) => {
    // Sidebar should have navigation items
    await expect(page.locator("text=Workflows")).toBeVisible();
    await expect(page.locator("text=Routes")).toBeVisible();
    await expect(page.locator("text=Services")).toBeVisible();

    // Toolbar should be present
    await expect(page.locator("button[title*='Undo']")).toBeVisible();
    await expect(page.locator("button[title*='Redo']")).toBeVisible();
    await expect(page.locator("button[title*='Save']")).toBeVisible();
  });

  test("shows empty state when no workflow is selected", async ({ page }) => {
    await expect(page.locator("text=Select a workflow")).toBeVisible();
  });

  test("navigates between views", async ({ page }) => {
    // Click on Routes view
    await page.locator("text=Routes").click();
    await expect(page.locator("text=Routes").first()).toBeVisible();

    // Click on Services view
    await page.locator("text=Services").click();
    await expect(page.locator("text=Services").first()).toBeVisible();

    // Click back to Workflows
    await page.locator("text=Workflows").first().click();
    await expect(page.locator("text=Select a workflow")).toBeVisible();
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

  test("export buttons are disabled when no workflow is loaded", async ({ page }) => {
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
