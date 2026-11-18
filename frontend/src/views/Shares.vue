<template>
  <div class="dashboard shares-page">
    <header-bar showMenu showLogo><div class="header-spacer" /></header-bar>
    <div class="workspace-heading">
      <div>
        <h1>{{ t("sidebar.shares") }}</h1>
        <p>{{ t("shares.description") }}</p>
      </div>
      <button
        class="button button--flat"
        :disabled="layoutStore.loading || busy"
        @click="reload"
      >
        {{ t("shares.refresh") }}
      </button>
    </div>
    <errors v-if="error" :errorCode="error.status" />
    <p v-else-if="layoutStore.loading" class="message" role="status">
      {{ t("files.loading") }}
    </p>
    <div v-else class="row single-column">
      <div class="column">
        <form
          v-if="editing"
          class="card share-editor"
          @submit.prevent="save"
          aria-labelledby="share-edit-title"
        >
          <div class="card-title">
            <h2 id="share-edit-title">{{ t("shares.edit") }}</h2>
          </div>
          <div class="card-content">
            <p class="share-path">{{ editing.path }}</p>
            <p class="small">{{ t("shares.stableLink") }}</p>
            <p>
              <label for="edit-share-password-mode">{{
                t("login.password")
              }}</label>
              <select
                id="edit-share-password-mode"
                ref="firstEditControl"
                class="input input--block"
                v-model="passwordMode"
                :disabled="busy"
              >
                <option value="keep">{{ t("shares.keepPassword") }}</option>
                <option value="set">{{ t("shares.setPassword") }}</option>
                <option value="remove">{{ t("shares.removePassword") }}</option>
              </select>
            </p>
            <p v-if="passwordMode === 'set'">
              <label for="edit-share-password">{{
                t("shares.newPassword")
              }}</label>
              <input
                id="edit-share-password"
                class="input input--block"
                type="password"
                autocomplete="new-password"
                v-model="password"
                required
                :maxlength="72"
                :disabled="busy"
              />
            </p>
            <p class="small" v-if="passwordMode === 'set'">
              {{ t("shares.passwordChangeHint") }}
            </p>
            <p>
              <label for="edit-share-expiration">{{
                t("settings.shareDuration")
              }}</label>
              <select
                id="edit-share-expiration"
                class="input input--block"
                v-model="expirationMode"
                :disabled="busy"
              >
                <option value="keep">{{ t("shares.keepExpiration") }}</option>
                <option value="permanent">{{ t("permanent") }}</option>
                <option value="duration">{{ t("shares.expireAfter") }}</option>
              </select>
            </p>
            <div v-if="expirationMode === 'duration'">
              <label for="edit-share-duration">{{
                t("shares.durationFromSave")
              }}</label>
              <div class="share-duration">
                <input
                  id="edit-share-duration"
                  class="input"
                  type="number"
                  required
                  min="1"
                  step="1"
                  :max="2147483647"
                  v-model.number="duration"
                  :disabled="busy"
                />
                <select
                  class="input right"
                  v-model="unit"
                  :aria-label="t('time.unit')"
                  :disabled="busy"
                >
                  <option value="seconds">{{ t("time.seconds") }}</option>
                  <option value="minutes">{{ t("time.minutes") }}</option>
                  <option value="hours">{{ t("time.hours") }}</option>
                  <option value="days">{{ t("time.days") }}</option>
                </select>
              </div>
            </div>
          </div>
          <div class="card-action">
            <button
              type="button"
              class="button button--flat button--grey"
              @click="closeEditor"
              :disabled="busy"
            >
              {{ t("buttons.cancel") }}
            </button>
            <button
              id="save-share"
              type="submit"
              class="button button--flat"
              :disabled="
                busy || (passwordMode === 'keep' && expirationMode === 'keep')
              "
            >
              {{ t("buttons.save") }}
            </button>
          </div>
        </form>
        <div class="card">
          <div class="card-title">
            <h2>{{ t("settings.shareManagement") }}</h2>
            <span class="small">{{
              t("shares.count", { count: links.length })
            }}</span>
          </div>
          <div class="card-content full" v-if="links.length">
            <table class="shares-table">
              <thead>
                <tr>
                  <th>{{ t("settings.path") }}</th>
                  <th>{{ t("settings.shareDuration") }}</th>
                  <th>{{ t("login.password") }}</th>
                  <th v-if="authStore.user?.perm.admin">
                    {{ t("settings.username") }}
                  </th>
                  <th>
                    <span class="sr-only">{{ t("shares.actions") }}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="link in links"
                  :key="link.hash"
                  :data-share-hash="link.hash"
                >
                  <td class="share-path">
                    <a
                      :href="api.getShareURL(link)"
                      target="_blank"
                      rel="noopener noreferrer"
                      >{{ link.path }}</a
                    >
                  </td>
                  <td
                    :title="
                      link.expire
                        ? dayjs(link.expire * 1000).format('YYYY-MM-DD HH:mm')
                        : undefined
                    "
                  >
                    {{
                      link.expire
                        ? dayjs(link.expire * 1000).fromNow()
                        : t("permanent")
                    }}
                  </td>
                  <td>
                    {{
                      t(
                        link.hasPassword
                          ? "shares.protected"
                          : "shares.noPassword"
                      )
                    }}
                  </td>
                  <td v-if="authStore.user?.perm.admin">{{ link.username }}</td>
                  <td class="share-actions">
                    <button
                      class="action"
                      @click="edit(link, $event)"
                      :disabled="busy"
                      :aria-label="t('shares.edit')"
                      :title="t('shares.edit')"
                    >
                      <i class="material-icons">edit</i>
                    </button>
                    <button
                      class="action copy-clipboard"
                      @click="copyLink(link)"
                      :aria-label="t('buttons.copyToClipboard')"
                      :title="t('buttons.copyToClipboard')"
                    >
                      <i class="material-icons">content_paste</i>
                    </button>
                    <button
                      class="action"
                      @click="deleteLink(link)"
                      :disabled="busy"
                      :aria-label="t('buttons.delete')"
                      :title="t('buttons.delete')"
                    >
                      <i class="material-icons">delete</i>
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p class="message" v-else>{{ t("shares.empty") }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useAuthStore } from "@/stores/auth";
import { useLayoutStore } from "@/stores/layout";
import { share as api, users } from "@/api";
import type { ShareUpdate } from "@/api/share";
import dayjs from "dayjs";
import Errors from "@/views/Errors.vue";
import HeaderBar from "@/components/header/HeaderBar.vue";
import { inject, ref, onMounted, nextTick } from "vue";
import { useI18n } from "vue-i18n";
import { StatusError } from "@/api/utils";
import { copy } from "@/utils/clipboard";

const showError = inject<IToastError>("$showError")!;
const showSuccess = inject<IToastSuccess>("$showSuccess")!;
const { t } = useI18n();
const layoutStore = useLayoutStore();
const authStore = useAuthStore();
const error = ref<StatusError | null>(null);
const links = ref<Share[]>([]);
const editing = ref<Share | null>(null);
const busy = ref(false);
const passwordMode = ref("keep");
const password = ref("");
const expirationMode = ref("keep");
const duration = ref(1);
const unit = ref("days");
const firstEditControl = ref<HTMLSelectElement>();
let editTrigger: HTMLElement | null = null;

async function reload() {
  closeEditor();
  error.value = null;
  layoutStore.loading = true;
  try {
    const fresh = await api.list();
    if (authStore.user?.perm.admin) {
      const names = new Map(
        (await users.getAll()).map((user) => [user.id, user.username])
      );
      for (const link of fresh)
        link.username = names.get(link.userID!) || t("shares.deletedUser");
    }
    links.value = fresh;
  } catch (err) {
    if (err instanceof Error) error.value = err;
  } finally {
    layoutStore.loading = false;
  }
}

async function edit(link: Share, event: Event) {
  editTrigger = event.currentTarget as HTMLElement;
  editing.value = link;
  passwordMode.value = "keep";
  password.value = "";
  expirationMode.value = "keep";
  duration.value = 1;
  unit.value = "days";
  await nextTick();
  firstEditControl.value?.focus();
}

function closeEditor() {
  editing.value = null;
  password.value = "";
  void nextTick(() => editTrigger?.focus());
}

async function save() {
  if (!editing.value || busy.value) return;
  const changes: ShareUpdate = {};
  if (passwordMode.value === "set") {
    if (!password.value) {
      showError(t("shares.passwordRequired"));
      return;
    }
    if (new TextEncoder().encode(password.value).length > 72) {
      showError(t("shares.passwordTooLong"));
      return;
    }
    changes.password = password.value;
  } else if (passwordMode.value === "remove") changes.password = "";
  if (expirationMode.value === "permanent") changes.expires = "";
  if (expirationMode.value === "duration") {
    changes.expires = String(duration.value);
    changes.unit = unit.value;
  }
  busy.value = true;
  try {
    const updated = await api.update(editing.value.hash, changes);
    links.value = links.value.map((link) =>
      link.hash === updated.hash ? { ...link, ...updated } : link
    );
    closeEditor();
    showSuccess(t("shares.updated"));
  } catch (err) {
    showError(err instanceof Error ? err : String(err));
  } finally {
    busy.value = false;
  }
}

async function copyLink(link: Share) {
  try {
    try {
      await copy({ text: api.getShareURL(link) });
    } catch {
      await copy({ text: api.getShareURL(link) }, { permission: true });
    }
    showSuccess(t("success.linkCopied"));
  } catch (err) {
    showError(err instanceof Error ? err : String(err));
  }
}

function deleteLink(link: Share) {
  layoutStore.showHover({
    prompt: "share-delete",
    confirm: async () => {
      layoutStore.closeHovers();
      busy.value = true;
      try {
        await api.remove(link.hash);
        links.value = links.value.filter((item) => item.hash !== link.hash);
        if (editing.value?.hash === link.hash) closeEditor();
        showSuccess(t("settings.shareDeleted"));
      } catch (err) {
        showError(err instanceof Error ? err : String(err));
      } finally {
        busy.value = false;
      }
    },
  });
}

onMounted(reload);
</script>

<style scoped>
.workspace-heading > div {
  min-width: 0;
  max-width: 100%;
}
.shares-page .column {
  flex-direction: column;
  min-width: 0;
}
.shares-page .column > .card {
  width: 100%;
  min-width: 0;
}
.share-path {
  overflow-wrap: anywhere;
}
.shares-table {
  min-width: 540px;
}
.share-actions {
  white-space: nowrap;
  text-align: end;
}
.share-actions .action {
  display: inline-flex;
  padding: 0.5em;
  min-width: 44px;
  min-height: 44px;
}
.share-editor {
  margin-bottom: 1.25rem;
}
.share-duration {
  display: flex;
  gap: 0.5rem;
}
.share-duration > input {
  flex: 1;
  width: 0;
}
.share-duration > select {
  max-width: 50%;
}
</style>
