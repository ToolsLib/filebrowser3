<template>
  <header>
    <router-link v-if="showLogo" class="brand" to="/files/" :aria-label="name">
      <img :src="logoURL" alt="" />
      <span>{{ name }}</span>
    </router-link>
    <Action
      v-if="showMenu"
      class="menu-button"
      icon="menu"
      :label="t('buttons.toggleSidebar')"
      @action="layoutStore.showHover('sidebar')"
      :aria-expanded="layoutStore.currentPromptName === 'sidebar'"
      aria-controls="workspace-navigation"
    />

    <slot />

    <div
      id="dropdown"
      :class="{ active: layoutStore.currentPromptName === 'more' }"
    >
      <slot name="actions" />
    </div>

    <Action
      v-if="ifActionsSlot"
      id="more"
      icon="more_vert"
      :label="t('buttons.more')"
      @action="layoutStore.showHover('more')"
      :aria-expanded="layoutStore.currentPromptName === 'more'"
      aria-controls="dropdown"
    />

    <div
      class="overlay"
      v-show="layoutStore.currentPromptName == 'more'"
      @click="layoutStore.closeHovers"
    />
  </header>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/stores/layout";

import { logoURL, name } from "@/utils/constants";

import Action from "@/components/header/Action.vue";
import { computed, useSlots, onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";

defineProps<{
  showLogo?: boolean;
  showMenu?: boolean;
}>();

const layoutStore = useLayoutStore();
const slots = useSlots();

const { t } = useI18n();

const ifActionsSlot = computed(() => (slots.actions ? true : false));
const closeMenu = (event: KeyboardEvent) => {
  if (
    event.key !== "Escape" ||
    !["more", "sidebar"].includes(layoutStore.currentPromptName ?? "")
  )
    return;
  const selector =
    layoutStore.currentPromptName === "more" ? "#more" : "header .menu-button";
  layoutStore.closeHovers();
  document.querySelector<HTMLElement>(selector)?.focus();
};
onMounted(() => window.addEventListener("keydown", closeMenu));
onUnmounted(() => window.removeEventListener("keydown", closeMenu));
</script>

<style></style>
