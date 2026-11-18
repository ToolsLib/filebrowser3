<template>
  <input
    :id="id"
    class="input input--block"
    type="text"
    required
    :value="text"
    placeholder="100 GiB"
    @input="update"
  />
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { formatUploadSize, parseUploadSize } from "@/utils/upload-limits";

const props = defineProps<{ id: string; modelValue?: number }>();
const emit = defineEmits<{ "update:modelValue": [value: number] }>();
const { t } = useI18n();
function editableSize(value: number) {
  const formatted = formatUploadSize(value);
  return parseUploadSize(formatted) === value ? formatted : `${value} B`;
}
const text = ref(editableSize(props.modelValue || 0));
watch(
  () => props.modelValue,
  (value) => {
    if (parseUploadSize(text.value) !== (value || 0))
      text.value = editableSize(value || 0);
  }
);
function update(event: Event) {
  const input = event.target as HTMLInputElement;
  text.value = input.value;
  const bytes = parseUploadSize(text.value);
  input.setCustomValidity(
    bytes === null ? t("settings.uploadLimitInvalid") : ""
  );
  if (bytes !== null) emit("update:modelValue", bytes);
}
</script>
