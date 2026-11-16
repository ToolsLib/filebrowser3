import { theme } from "./constants";

export const getTheme = (): UserTheme => {
  return (document.documentElement.className as UserTheme) || theme;
};

export const setTheme = (theme: UserTheme) => {
  const html = document.documentElement;
  if (!theme) {
    html.className = getMediaPreference();
  } else {
    html.className = theme;
  }
};

export const toggleTheme = (): void => {
  const activeTheme = getTheme();
  if (activeTheme === "light") {
    setTheme("dark");
  } else {
    setTheme("light");
  }
};

export const getMediaPreference = (): UserTheme => {
  const hasDarkPreference = window.matchMedia(
    "(prefers-color-scheme: dark)"
  ).matches;
  if (hasDarkPreference) {
    return "dark";
  } else {
    return "light";
  }
};

export const getEditorTheme = (
  themeName: string,
  themesByName: Record<string, unknown>
) => {
  if (!themeName.startsWith("ace/theme/")) {
    themeName = `ace/theme/${themeName}`;
  }
  const themeKey = themeName.replace("ace/theme/", "");
  if (themesByName[themeKey] !== undefined) {
    return themeName;
  } else if (getTheme() === "dark") {
    return "ace/theme/twilight";
  } else {
    return "ace/theme/chrome";
  }
};
