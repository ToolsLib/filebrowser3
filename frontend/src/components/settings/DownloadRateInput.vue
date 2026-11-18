<template>
  <select :id="`${id}-mode`" v-model="mode" class="input input--block">
    <option v-if="inheritable" value="inherit">
      {{ t("settings.downloadInherit") }}
    </option>
    <option value="limited">{{ t("settings.downloadCustom") }}</option>
    <option value="unlimited">{{ t("settings.downloadUnlimited") }}</option>
  </select>
  <span v-if="mode === 'limited'">
    <label :for="id">{{ t("settings.downloadRateValue") }}</label>
    <input
      :id="id"
      class="input input--block"
      type="text"
      required
      :value="text"
      placeholder="10 MiB/s"
      @input="update"
    />
  </span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { formatDownloadRate, parseDownloadRate } from "@/utils/download-limits";

const props = defineProps<{
  id: string;
  modelValue?: number | null;
  inheritable?: boolean;
}>();
const emit = defineEmits<{ "update:modelValue": [value: number | null] }>();
const { t } = useI18n();
const text = ref(formatDownloadRate(props.modelValue || 1024 ** 2));
const mode = computed({
  get: () =>
    props.inheritable && props.modelValue == null
      ? "inherit"
      : props.modelValue
        ? "limited"
        : "unlimited",
  set: (value: string) =>
    emit(
      "update:modelValue",
      value === "inherit"
        ? null
        : value === "unlimited"
          ? 0
          : props.modelValue || 1024 ** 2
    ),
});
watch(
  () => props.modelValue,
  (value) => {
    if (value && parseDownloadRate(text.value) !== value)
      text.value = formatDownloadRate(value);
  }
);
function update(event: Event) {
  const input = event.target as HTMLInputElement;
  text.value = input.value;
  const rate = parseDownloadRate(text.value);
  input.setCustomValidity(
    rate === null || rate === 0 ? t("settings.downloadRateInvalid") : ""
  );
  if (rate !== null && rate > 0) emit("update:modelValue", rate);
}
</script>
