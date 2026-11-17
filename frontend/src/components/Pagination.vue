<template>
  <div
    v-if="hasNext !== undefined ? page > 1 || hasNext : total > pageSize"
    class="pagination"
    :aria-label="$t('ui.pagination')"
  >
    <button
      class="button button--flat"
      :disabled="page <= 1"
      @click="$emit('change', page - 1)"
    >
      {{ $t("buttons.previous") }}
    </button>
    <span role="status">{{
      hasNext !== undefined
        ? $t("ui.page", { page })
        : $t("ui.pageOf", { page, pages: Math.ceil(total / pageSize) })
    }}</span>
    <button
      class="button button--flat"
      :disabled="
        hasNext !== undefined ? !hasNext : page >= Math.ceil(total / pageSize)
      "
      @click="$emit('change', page + 1)"
    >
      {{ $t("buttons.next") }}
    </button>
  </div>
</template>
<script setup lang="ts">
withDefaults(
  defineProps<{
    page: number;
    total: number;
    pageSize: number;
    hasNext?: boolean;
  }>(),
  { hasNext: undefined }
);
defineEmits<{ change: [page: number] }>();
</script>
<style scoped>
.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 16px 0;
  flex-wrap: wrap;
  font-size: 13px;
  color: var(--textPrimary);
}
</style>
