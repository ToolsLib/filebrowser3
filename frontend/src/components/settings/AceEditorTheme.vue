<template>
  <select
    name="selectAceEditorTheme"
    v-on:change="change"
    :value="aceEditorTheme"
  >
    <option value="">{{ $t("ui.defaultTheme") }}</option>
    <option v-for="theme in themes" :value="theme.theme" :key="theme.theme">
      {{ theme.caption }}
    </option>
  </select>
</template>

<script setup lang="ts">
import { type SelectHTMLAttributes } from "vue";
import { themes } from "virtual:ace-themes";

defineProps<{
  aceEditorTheme: string;
}>();

const emit = defineEmits<{
  (e: "update:aceEditorTheme", val: string | null): void;
}>();

const change = (event: Event) => {
  emit("update:aceEditorTheme", (event.target as SelectHTMLAttributes)?.value);
};
</script>
