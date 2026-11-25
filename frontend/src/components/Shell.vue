<template>
  <div
    class="shell"
    :class="{ ['shell--hidden']: !showShell }"
    :style="{ height: `${this.shellHeight}em`, direction: 'ltr' }"
    @keydown.esc="stopCommand"
  >
    <div
      @pointerdown="startDrag()"
      @pointerup="stopDrag()"
      class="shell__divider"
      :style="this.shellDrag ? { background: `${checkTheme()}` } : ''"
    ></div>
    <button
      v-if="!canInput"
      type="button"
      class="shell__cancel button button--flat"
      @click="stopCommand"
    >
      {{ $t("buttons.cancel") }}
    </button>
    <div @click="focus" class="shell__content" ref="scrollable">
      <div v-for="(c, index) in content" :key="index" class="shell__result">
        <div class="shell__prompt">
          <i class="material-icons">chevron_right</i>
        </div>
        <pre class="shell__text">{{ c.text }}</pre>
      </div>

      <div
        class="shell__result"
        :class="{ 'shell__result--hidden': !canInput }"
      >
        <div class="shell__prompt">
          <i class="material-icons">chevron_right</i>
        </div>
        <pre
          tabindex="0"
          ref="input"
          class="shell__text"
          :contenteditable="true"
          @keydown.prevent.arrow-up="historyUp"
          @keydown.prevent.arrow-down="historyDown"
          @keypress.prevent.enter="submit"
        />
      </div>
    </div>
    <div
      @pointerup="stopDrag()"
      class="shell__overlay"
      v-show="this.shellDrag"
    ></div>
  </div>
</template>

<script>
import { reactive } from "vue";
import { mapState, mapActions } from "pinia";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";

import { commands } from "@/api";
import { throttle } from "lodash-es";
import { theme } from "@/utils/constants";

export default {
  name: "shell",
  computed: {
    ...mapState(useLayoutStore, ["showShell"]),
    ...mapState(useFileStore, ["isFiles"]),
    path: function () {
      if (this.isFiles) {
        return this.$route.path;
      }

      return "";
    },
  },
  data: () => ({
    content: [],
    history: [],
    historyPos: 0,
    canInput: true,
    shellDrag: false,
    shellHeight: 25,
    fontsize: parseFloat(getComputedStyle(document.documentElement).fontSize),
    commandController: null,
    disposed: false,
  }),
  watch: {
    showShell(visible) {
      if (!visible) this.stopCommand();
    },
    "$route.path"() {
      this.stopCommand();
    },
  },
  mounted() {
    window.addEventListener("resize", this.resize);
  },
  beforeUnmount() {
    this.disposed = true;
    this.stopCommand();
    window.removeEventListener("resize", this.resize);
    this.stopDrag();
  },
  methods: {
    ...mapActions(useLayoutStore, ["toggleShell"]),
    checkTheme() {
      if (theme == "dark") {
        return "rgba(255, 255, 255, 0.4)";
      }
      return "rgba(127, 127, 127, 0.4)";
    },
    startDrag() {
      document.addEventListener("pointermove", this.handleDrag);
      this.shellDrag = true;
    },
    stopDrag() {
      document.removeEventListener("pointermove", this.handleDrag);
      this.shellDrag = false;
    },
    handleDrag: throttle(function (event) {
      const top = window.innerHeight / this.fontsize - 4;
      const userPos = (window.innerHeight - event.clientY) / this.fontsize;
      const bottom =
        2.25 +
        document.querySelector(".shell__divider").offsetHeight / this.fontsize;

      if (userPos <= top && userPos >= bottom) {
        this.shellHeight = userPos.toFixed(2);
      }
    }, 32),
    resize: throttle(function () {
      const top = window.innerHeight / this.fontsize - 4;
      const bottom =
        2.25 +
        document.querySelector(".shell__divider").offsetHeight / this.fontsize;

      if (this.shellHeight > top) {
        this.shellHeight = top;
      } else if (this.shellHeight < bottom) {
        this.shellHeight = bottom;
      }
    }, 32),
    scroll: function () {
      const container = this.$refs.scrollable;
      if (container) container.scrollTop = container.scrollHeight;
    },
    focus: function () {
      this.$refs.input.focus();
    },
    historyUp() {
      if (this.historyPos > 0) {
        this.$refs.input.innerText = this.history[--this.historyPos];
        this.focus();
      }
    },
    historyDown() {
      if (this.historyPos >= 0 && this.historyPos < this.history.length - 1) {
        this.$refs.input.innerText = this.history[++this.historyPos];
        this.focus();
      } else {
        this.historyPos = this.history.length;
        this.$refs.input.innerText = "";
      }
    },
    stopCommand() {
      this.commandController?.abort();
    },
    submit: async function (event) {
      if (!this.canInput) return;
      const cmd = event.target.innerText.trim();

      if (cmd === "") {
        return;
      }

      if (cmd === "clear") {
        this.content = [];
        event.target.innerHTML = "";
        return;
      }

      if (cmd === "exit") {
        event.target.innerHTML = "";
        this.toggleShell();
        return;
      }

      this.canInput = false;
      event.target.innerHTML = "";

      const results = reactive({
        text: `${cmd}\n\n`,
      });

      this.history.push(cmd);
      this.historyPos = this.history.length;
      this.content.push(results);
      const controller = new AbortController();
      this.commandController = controller;
      const append = (text) => {
        // Bound retained output even when a command writes continuously.
        results.text = (results.text + text).slice(-1024 * 1024);
        this.scroll();
      };
      try {
        const code = await commands(this.path, cmd, append, controller.signal);
        if (code !== 0) append(`\n${this.$t("shell.exitCode", { code })}\n`);
      } catch (error) {
        append(
          `\n${controller.signal.aborted ? this.$t("shell.cancelled") : error.message}\n`
        );
      } finally {
        this.commandController = null;
        results.text = results.text.replace(/\u001b\[[0-9;]+m/g, "").trimEnd();
        this.canInput = true;
        if (!this.disposed) {
          this.$refs.input?.focus();
          this.scroll();
        }
      }
    },
  },
};
</script>
