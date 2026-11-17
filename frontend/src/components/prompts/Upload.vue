<template>
  <div class="card floating">
    <div class="card-title">
      <h2>{{ t("prompts.upload") }}</h2>
    </div>

    <div class="card-content">
      <p>{{ t("prompts.uploadMessage") }}</p>
    </div>

    <div class="card-action full">
      <button
        type="button"
        @click="uploadFile"
        class="action"
        id="focus-prompt"
      >
        <i class="material-icons">insert_drive_file</i>
        <span class="title">{{ t("buttons.file") }}</span>
      </button>
      <button type="button" @click="uploadFolder" class="action">
        <i class="material-icons">folder</i>
        <span class="title">{{ t("buttons.folder") }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";
import { useLayoutStore } from "@/stores/layout";

import * as upload from "@/utils/upload";
import buttons from "@/utils/buttons";

const { t } = useI18n();
const route = useRoute();

const layoutStore = useLayoutStore();
let conflictController = new AbortController();
const $showError = inject<IToastError>("$showError")!;
onBeforeUnmount(() => conflictController.abort());

// TODO: this is a copy of the same function in FileListing.vue
const uploadInput = async (event: Event) => {
  const files = (event.currentTarget as HTMLInputElement)?.files;
  if (files === null) return;

  const folder_upload = !!files[0].webkitRelativePath;

  const uploadFiles: UploadList = [];
  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    const fullPath = folder_upload ? file.webkitRelativePath : undefined;
    uploadFiles.push({
      file,
      name: file.name,
      size: file.size,
      isDir: false,
      fullPath,
    });
  }

  const path = route.path.endsWith("/") ? route.path : route.path + "/";

  // Checking the destination hits the server, so show it is working rather
  // than leaving the action looking inert until the upload starts.
  buttons.loading("upload");
  conflictController.abort();
  conflictController = new AbortController();
  let conflict: ConflictingResource[];
  try {
    conflict = await upload.checkConflict(
      uploadFiles,
      path,
      false,
      conflictController.signal
    );
  } catch (error) {
    buttons.done("upload");
    if (!(
      error instanceof Error &&
      "is_canceled" in error &&
      error.is_canceled
    ))
      $showError(error as Error);
    return;
  }

  if (conflict.length > 0) {
    buttons.done("upload");
    layoutStore.showHover({
      prompt: "resolve-conflict",
      props: {
        conflict: conflict,
        isUploadAction: true,
      },
      confirm: (event: Event, result: Array<ConflictingResource>) => {
        event.preventDefault();
        layoutStore.closeHovers();
        for (let i = result.length - 1; i >= 0; i--) {
          const item = result[i];
          if (item.checked.length == 2) {
            continue;
          } else if (item.checked.length == 1 && item.checked[0] == "origin") {
            uploadFiles[item.index].overwrite = true;
          } else {
            uploadFiles.splice(item.index, 1);
          }
        }
        if (uploadFiles.length > 0) {
          upload.handleFiles(uploadFiles, path);
        }
      },
    });

    return;
  }

  upload.handleFiles(uploadFiles, path);
};

const openUpload = (isFolder: boolean) => {
  const input = document.createElement("input");
  input.type = "file";
  input.multiple = true;
  input.webkitdirectory = isFolder;
  // TODO: call the function in FileListing.vue instead
  input.onchange = uploadInput;
  input.click();
};

const uploadFile = () => {
  openUpload(false);
};
const uploadFolder = () => {
  openUpload(true);
};
</script>
