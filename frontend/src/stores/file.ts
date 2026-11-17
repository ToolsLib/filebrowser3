import { defineStore } from "pinia";

export const useFileStore = defineStore("file", {
  // convert to a function
  state: (): {
    req: Resource | null;
    oldReq: Resource | null;
    browseLocation: { path: string; page: number } | null;
    reload: boolean;
    selected: number[];
    multiple: boolean;
    isFiles: boolean;
    preselect: string | null;
  } => ({
    req: null,
    oldReq: null,
    browseLocation: null,
    reload: false,
    selected: [],
    multiple: false,
    isFiles: false,
    preselect: null,
  }),
  getters: {
    selectedCount: (state) => state.selected.length,
    selectedSet: (state) => new Set(state.selected),
    // route: () => {
    //   const routerStore = useRouterStore();
    //   return routerStore.router.currentRoute;
    // },
    // isFiles: (state) => {
    //   const layoutStore = useLayoutStore();
    //   return !layoutStore.loading && state.route._value.name === "Files";
    // },
    isListing: (state) => {
      return state.isFiles && state?.req?.isDir;
    },
  },
  actions: {
    // no context as first argument, use `this` instead
    toggleMultiple() {
      this.multiple = !this.multiple;
    },
    updateRequest(value: Resource | null) {
      const selectedUrls = new Set(
        this.selected.map((i) => this.req?.items[i]?.url)
      );
      this.oldReq = this.req;
      this.req = value;

      this.selected = [];

      if (!this.req?.items || selectedUrls.size === 0) return;
      this.selected = this.req.items
        .filter((item) => selectedUrls.has(item.url))
        .map((item) => item.index);
    },
    removeSelected(value: any) {
      const i = this.selected.indexOf(value);
      if (i === -1) return;
      this.selected.splice(i, 1);
    },
    // easily reset state using `$reset`
    clearFile() {
      this.$reset();
    },
  },
});
