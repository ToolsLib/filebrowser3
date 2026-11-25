import { expect, test } from "@playwright/test";
import { access, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

test("command HTTP stream renders incrementally, reports exits, and cancels on stop, close and navigation", async ({
  page,
}) => {
  const fixture = JSON.parse(
    await readFile("test-results/command-fixture.json", "utf8")
  );
  const quote = (value: string) => JSON.stringify(value.replaceAll("\\", "/"));
  const node = fixture.node.replaceAll("\\", "/");
  const command = (mode: string, marker: string) =>
    `${quote(node)} ${quote(fixture.helper)} ${mode} ${quote(marker)}`;
  const errors: string[] = [];
  const sockets: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("websocket", (socket) => sockets.push(socket.url()));
  await page.goto("login");
  await page.getByLabel("Username", { exact: true }).fill("reviewer");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Browser-test-password-42!");
  const loggedIn = page.waitForResponse(
    (response) => response.url().endsWith("/api/login") && response.ok()
  );
  await page.locator('input[type="submit"]').click();
  const headers = { "X-Auth": await (await loggedIn).text() };
  const user = await (
    await page.request.get("api/users/1", { headers })
  ).json();
  const saveCommands = async (commands: string[]) => {
    const response = await page.request.put("api/users/1", {
      headers,
      data: {
        what: "user",
        which: ["Commands"],
        current_password: "Browser-test-password-42!",
        data: { ...user, commands },
      },
    });
    expect(response.ok()).toBe(true);
  };
  await saveCommands([node]);
  try {
    await page.goto("files/Documents/");
    await page
      .getByRole("button", { name: "Toggle shell", exact: true })
      .click();
    const input = page.locator('.shell [contenteditable="true"]');
    const cancel = page
      .locator(".shell")
      .getByRole("button", { name: "Cancel", exact: true });
    const output = page.locator(".shell__content");
    const marker = path.join(fixture.directory, "release-command");
    const request = page.waitForRequest((req) =>
      req.url().includes("/api/command/")
    );
    await input.fill(command("wait", marker));
    await input.press("Enter");
    const sent = await request;
    expect(sent.method()).toBe("POST");
    expect(sent.headers()["x-auth"]).toBeTruthy();
    expect(new URL(sent.url()).pathname).toBe(
      "/test-app/api/command/Documents/"
    );
    expect(new URL(sent.url()).search).toBe("");
    await expect(output).toContainText("first 🌍 partial");
    await expect(cancel).toBeVisible();
    await writeFile(marker, "release");
    await expect(output).toContainText("first 🌍 partial second");
    await expect(cancel).toHaveCount(0);
    await input.fill(command("exit", marker));
    await input.press("Enter");
    await expect(output).toContainText("Command exited with status 7.");

    for (const action of ["cancel", "escape", "close", "navigate"]) {
      const escaped = path.join(fixture.directory, `escaped-${action}`);
      await input.fill(command("tree", escaped));
      await input.press("Enter");
      await expect(cancel).toBeVisible();
      // The new result must contain the child output, not only the command text.
      await expect(
        page.locator(".shell__result:has(pre:not([contenteditable]))").last()
      ).toContainText("child-ready");
      if (action === "cancel") await cancel.click();
      else if (action === "escape") await input.press("Escape");
      else if (action === "close")
        await page
          .getByRole("button", { name: "Toggle shell", exact: true })
          .click();
      else await page.getByRole("link", { name: "Home", exact: true }).click();
      await expect(output).toContainText("Command cancelled.");
      await expect(cancel).toHaveCount(0);
      await page.waitForTimeout(1700);
      await expect(access(escaped)).rejects.toThrow();
      if (action === "close")
        await page
          .getByRole("button", { name: "Toggle shell", exact: true })
          .click();
    }
    expect(sockets).toEqual([]);
    expect(errors).toEqual([]);
  } finally {
    await saveCommands(user.commands);
  }
});
