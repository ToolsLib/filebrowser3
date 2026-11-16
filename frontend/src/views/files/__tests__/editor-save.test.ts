import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Editor from "@/views/files/Editor.vue";

const harness = vi.hoisted(() => {
  const undo = { isClean: vi.fn(() => false), markClean: vi.fn() };
  return {
    undo,
    editor: {
      getValue: vi.fn(() => "unsaved buffer"),
      session: { getUndoManager: () => undo },
    },
    put: vi.fn(),
    error: vi.fn(),
    showHover: vi.fn(),
    guard: vi.fn(),
    navigate: vi.fn(),
  };
});

vi.mock("@/api", () => ({ files: { put: harness.put } }));
vi.mock("@/utils/buttons", () => ({
  default: { loading: vi.fn(), done: vi.fn(), success: vi.fn() },
}));
vi.mock("@/utils/url", () => ({ default: { removeLastDir: () => "/files" } }));
vi.mock("@/utils/theme", () => ({ getEditorTheme: () => "textmate" }));
vi.mock("@/utils/clipboard", () => ({ read: vi.fn(), copy: vi.fn() }));
vi.mock("ace-builds", () => ({ default: {}, version: "test" }));
vi.mock("ace-builds/src-noconflict/ext-language_tools", () => ({}));
vi.mock("ace-builds/src-noconflict/ext-modelist", () => ({ default: {} }));
vi.mock("ace-builds/src-noconflict/ext-themelist", () => ({ themesByName: {} }));
vi.mock("@/components/Breadcrumbs.vue", () => ({ default: {} }));
vi.mock("@/components/header/Action.vue", () => ({ default: {} }));
vi.mock("@/components/header/HeaderBar.vue", () => ({ default: {} }));
vi.mock("@/stores/auth", () => ({
  useAuthStore: () => ({ user: { perm: { modify: true } } }),
}));
vi.mock("@/stores/file", () => ({
  useFileStore: () => ({
    req: { name: "file.txt", type: "text", content: "original" },
  }),
}));
vi.mock("@/stores/layout", () => ({
  useLayoutStore: () => ({ showHover: harness.showHover, loading: false }),
}));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock("vue-router", () => ({
  onBeforeRouteUpdate: harness.guard,
  useRoute: () => ({ path: "/files/file.txt" }),
  useRouter: () => ({ push: harness.navigate }),
}));
vi.mock("vue", async (original) => {
  const vue = await original<typeof import("vue")>();
  return {
    ...vue,
    ref: (value: unknown) => vue.ref(value === null ? harness.editor : value),
    inject: () => harness.error,
    onMounted: vi.fn(),
    onBeforeUnmount: vi.fn(),
    watchEffect: vi.fn(),
    useSSRContext: () => ({ modules: new Set() }),
  };
});

describe("editor save before navigation", () => {
  afterEach(() => vi.unstubAllGlobals());
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("localStorage", { getItem: () => null });
    (Editor as any).setup({}, { expose: vi.fn() });
  });
  it("retains the buffer and navigation when saving fails, then permits retry", async () => {
    const next = vi.fn();
    harness.guard.mock.calls[0][0]({}, {}, next);
    const prompt = harness.showHover.mock.calls[0][0];
    harness.put.mockRejectedValueOnce(new Error("save failed"));
    await prompt.saveAction();
    expect(next).not.toHaveBeenCalled();
    expect(harness.undo.markClean).not.toHaveBeenCalled();
    expect(harness.editor.getValue()).toBe("unsaved buffer");
    expect(harness.error).toHaveBeenCalledOnce();
    harness.put.mockResolvedValueOnce(undefined);
    await prompt.saveAction();
    expect(next).toHaveBeenCalledOnce();
    expect(harness.undo.markClean).toHaveBeenCalledOnce();
  });
});
