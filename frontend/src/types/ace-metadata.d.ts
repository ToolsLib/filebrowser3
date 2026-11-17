declare module "virtual:ace-themes" {
  export const version: string;
  export const themes: {
    caption: string;
    name: string;
    theme: string;
    isDark: boolean;
  }[];
  export const themesByName: Record<string, { theme: string; isDark: boolean }>;
}
