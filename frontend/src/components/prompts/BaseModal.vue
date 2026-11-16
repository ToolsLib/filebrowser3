<template>
  <div id="modal-background" @click="backgroundClick">
    <div
      ref="modalContainer"
      class="dialog-container"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      tabindex="-1"
    >
      <slot></slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, useId, watch } from "vue";

const props = defineProps<{ prompt?: string | null }>();
const emit = defineEmits(["closed"]);

const modalContainer = ref<HTMLElement | null>(null);
const titleId = useId();
const previousFocus = document.activeElement as HTMLElement | null;
const previousOverflow = document.body.style.overflow;
const focusable = () =>
  Array.from(
    modalContainer.value?.querySelectorAll<HTMLElement>(
      'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex="0"]'
    ) ?? []
  ).filter((el) => el.getClientRects().length > 0);

const focusDialog = async () => {
  await nextTick();
  const heading = modalContainer.value?.querySelector("h2");
  if (heading) heading.id = titleId;
  const target =
    modalContainer.value?.querySelector<HTMLElement>("#focus-prompt");
  (target ?? focusable()[0] ?? modalContainer.value)?.focus();
};
const keydown = (event: KeyboardEvent) => {
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopImmediatePropagation();
    emit("closed");
  } else if (event.key === "Tab") {
    const elements = focusable();
    const first = elements[0],
      last = elements[elements.length - 1];
    if (!first) {
      event.preventDefault();
      modalContainer.value?.focus();
    } else if (
      event.shiftKey &&
      (document.activeElement === first ||
        !elements.includes(document.activeElement as HTMLElement))
    ) {
      event.preventDefault();
      last.focus();
    } else if (
      !event.shiftKey &&
      (document.activeElement === last ||
        !elements.includes(document.activeElement as HTMLElement))
    ) {
      event.preventDefault();
      first.focus();
    }
  }
};
onMounted(() => {
  document.body.style.overflow = "hidden";
  window.addEventListener("keydown", keydown, true);
  focusDialog();
});
onUnmounted(() => {
  window.removeEventListener("keydown", keydown, true);
  document.body.style.overflow = previousOverflow;
  if (previousFocus?.isConnected) previousFocus.focus();
});
watch(() => props.prompt, focusDialog);

const backgroundClick = (event: Event) => {
  const target = event.target as HTMLElement;
  if (target.id == "modal-background") {
    emit("closed");
  }
};
</script>

<style scoped>
.dialog-container {
  width: min(460px, 90vw);
}
.dialog-container :deep(.card.floating) {
  position: relative;
  inset: auto;
  transform: none;
  width: 100%;
  max-width: none;
  max-height: 90dvh;
  margin: 0;
}
#modal-background {
  position: fixed;
  inset: 0;
  background-color: #12182070;
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 10000;
  animation: ease-in 150ms opacity-enter;
}

@keyframes opacity-enter {
  from {
    opacity: 0;
  }

  to {
    opacity: 1;
  }
}
</style>
