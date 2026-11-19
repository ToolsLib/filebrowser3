<template>
  <div ref="container" class="new-menu" @keydown="keydown" @focusout="focusout">
    <button
      ref="trigger"
      type="button"
      class="button button--flat new-button"
      :aria-expanded="open"
      :aria-controls="menuId"
      aria-haspopup="menu"
      @click="toggle"
    >
      <i class="material-icons" aria-hidden="true">add</i>
      <span>{{ t("buttons.new") }}</span>
      <i class="material-icons" aria-hidden="true">expand_more</i>
    </button>
    <div
      v-if="open"
      :id="menuId"
      ref="menu"
      class="new-menu-panel"
      role="menu"
      :aria-label="t('buttons.new')"
    >
      <button
        type="button"
        role="menuitem"
        tabindex="-1"
        @click="create('newDir')"
      >
        <i class="material-icons" aria-hidden="true">create_new_folder</i>
        <span>{{ t("sidebar.newFolder") }}</span>
      </button>
      <button
        type="button"
        role="menuitem"
        tabindex="-1"
        @click="create('newFile')"
      >
        <i class="material-icons" aria-hidden="true">note_add</i>
        <span>{{ t("sidebar.newFile") }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, useId, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useLayoutStore } from "@/stores/layout";

const { t } = useI18n();
const layoutStore = useLayoutStore();
const menuId = useId();
const container = ref<HTMLElement>();
const trigger = ref<HTMLButtonElement>();
const menu = ref<HTMLElement>();
const open = ref(false);

function close(restoreFocus = false) {
  open.value = false;
  if (restoreFocus) trigger.value?.focus();
}
async function show(last = false) {
  open.value = true;
  await nextTick();
  const items = menu.value?.querySelectorAll<HTMLButtonElement>("button");
  items?.[last ? items.length - 1 : 0]?.focus();
}
function toggle() {
  if (open.value) close(true);
  else void show();
}
function create(prompt: "newDir" | "newFile") {
  // The modal remembers the visible trigger, rather than a hidden menu item.
  close(true);
  layoutStore.showHover(prompt);
}
function keydown(event: KeyboardEvent) {
  if (!open.value) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      void show(event.key === "ArrowUp");
    }
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopPropagation();
    close(true);
  } else if (event.key === "Tab") close();
  else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
    event.preventDefault();
    const items = Array.from(
      menu.value?.querySelectorAll<HTMLButtonElement>("button") ?? []
    );
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    const index =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? items.length - 1
          : (current + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
            items.length;
    items[index]?.focus();
  }
}
function pointerdown(event: PointerEvent) {
  if (event.target instanceof Node && !container.value?.contains(event.target))
    close();
}
function focusout(event: FocusEvent) {
  if (
    !(event.relatedTarget instanceof Node) ||
    !container.value?.contains(event.relatedTarget)
  )
    close();
}
watch(
  () => layoutStore.currentPromptName,
  () => close()
);
onMounted(() => document.addEventListener("pointerdown", pointerdown));
onUnmounted(() => document.removeEventListener("pointerdown", pointerdown));
</script>

<style scoped>
.new-menu {
  position: relative;
  flex-shrink: 0;
}
.new-button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--blue);
  background: var(--surfacePrimary);
  color: var(--blue);
}
.material-icons {
  font-size: 20px;
}
.new-menu-panel {
  position: absolute;
  inset-inline-end: 0;
  top: calc(100% + 8px);
  z-index: 100;
  min-width: 180px;
  max-width: calc(100vw - 40px);
  padding: 6px;
  border: 1px solid var(--borderPrimary);
  border-radius: 9px;
  background: var(--surfacePrimary);
  box-shadow: 0 8px 24px #00000020;
}
.new-menu-panel button {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  min-height: 44px;
  padding: 10px 12px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--textSecondary);
  text-align: start;
  cursor: pointer;
}
.new-menu-panel button:hover,
.new-menu-panel button:focus-visible {
  background: var(--surfaceSecondary);
}
</style>
