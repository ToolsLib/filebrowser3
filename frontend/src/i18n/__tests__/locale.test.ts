import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { detectLocale, i18n, isRtl, setLocale } from "../index";
import dayjs from "dayjs";

describe("lazy language selection", () => {
  beforeEach(() =>
    vi.stubGlobal("document", { documentElement: { lang: "en", dir: "ltr" } })
  );
  afterEach(() => vi.unstubAllGlobals());
  it.each([
    ["nl-BE", "nl-be"],
    ["sv-SE", "sv-se"],
    ["ca-ES", "ca"],
    ["fa-IR", "fa"],
    ["pt-PT", "pt-pt"],
    ["nb-NO", "no"],
  ])("detects %s as %s", (language, locale) => {
    vi.stubGlobal("navigator", { language });
    expect(detectLocale()).toBe(locale);
  });
  it("loads messages and date locale, and applies only the latest selection", async () => {
    await Promise.all([setLocale("fr"), setLocale("he")]);
    expect(i18n.global.locale.value).toBe("he");
    expect(dayjs.locale()).toBe("he");
    expect(i18n.global.t("buttons.save")).not.toBe("buttons.save");
    expect(isRtl()).toBe(true);
    expect(document.documentElement.dir).toBe("rtl");
    await setLocale("unknown");
    expect(i18n.global.locale.value).toBe("en");
    expect(i18n.global.t("buttons.save")).toBe("Save");
  });
});
