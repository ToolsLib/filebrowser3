import dayjs from "dayjs";
import { createI18n } from "vue-i18n";

import english from "./en.json";

const messageLoaders = import.meta.glob<{ default: typeof english }>([
  "./*.json",
  "!./en.json",
]);
const dateLoaders: Record<string, () => Promise<unknown>> = {
  ar: () => import("dayjs/locale/ar.js"),
  bg: () => import("dayjs/locale/bg.js"),
  ca: () => import("dayjs/locale/ca.js"),
  cs: () => import("dayjs/locale/cs.js"),
  de: () => import("dayjs/locale/de.js"),
  el: () => import("dayjs/locale/el.js"),
  en: () => import("dayjs/locale/en.js"),
  es: () => import("dayjs/locale/es.js"),
  fa: () => import("dayjs/locale/fa.js"),
  fr: () => import("dayjs/locale/fr.js"),
  he: () => import("dayjs/locale/he.js"),
  hr: () => import("dayjs/locale/hr.js"),
  hu: () => import("dayjs/locale/hu.js"),
  is: () => import("dayjs/locale/is.js"),
  it: () => import("dayjs/locale/it.js"),
  ja: () => import("dayjs/locale/ja.js"),
  ko: () => import("dayjs/locale/ko.js"),
  lv: () => import("dayjs/locale/lv.js"),
  "nl-be": () => import("dayjs/locale/nl-be.js"),
  nl: () => import("dayjs/locale/nl.js"),
  no: () => import("dayjs/locale/nb.js"),
  pl: () => import("dayjs/locale/pl.js"),
  "pt-br": () => import("dayjs/locale/pt-br.js"),
  "pt-pt": () => import("dayjs/locale/pt.js"),
  ro: () => import("dayjs/locale/ro.js"),
  ru: () => import("dayjs/locale/ru.js"),
  sk: () => import("dayjs/locale/sk.js"),
  "sv-se": () => import("dayjs/locale/sv.js"),
  tr: () => import("dayjs/locale/tr.js"),
  uk: () => import("dayjs/locale/uk.js"),
  vi: () => import("dayjs/locale/vi.js"),
  "zh-cn": () => import("dayjs/locale/zh-cn.js"),
  "zh-tw": () => import("dayjs/locale/zh-tw.js"),
};

export function detectLocale() {
  // locale is an RFC 5646 language tag
  // https://developer.mozilla.org/en-US/docs/Web/API/Navigator/language
  let locale = navigator.language.toLowerCase();
  switch (true) {
    case /^ar\b/.test(locale):
      locale = "ar";
      break;
    case /^bg\b/.test(locale):
      locale = "bg";
      break;
    case /^cs\b/.test(locale):
      locale = "cs";
      break;
    case /^lv\b/.test(locale):
      locale = "lv";
      break;
    case /^he\b/.test(locale):
      locale = "he";
      break;
    case /^hr\b/.test(locale):
      locale = "hr";
      break;
    case /^hu\b/.test(locale):
      locale = "hu";
      break;
    case /^el.*/i.test(locale):
      locale = "el";
      break;
    case /^es\b/.test(locale):
      locale = "es";
      break;
    case /^en\b/.test(locale):
      locale = "en";
      break;
    case /^is\b/.test(locale):
      locale = "is";
      break;
    case /^it\b/.test(locale):
      locale = "it";
      break;
    case /^fr\b/.test(locale):
      locale = "fr";
      break;
    case /^pt-br\b/.test(locale):
      locale = "pt-br";
      break;
    case /^pt-pt\b/.test(locale):
    case /^pt\b/.test(locale):
      locale = "pt-pt";
      break;
    case /^ja\b/.test(locale):
      locale = "ja";
      break;
    case /^zh-tw\b/.test(locale):
      locale = "zh-tw";
      break;
    case /^zh-cn\b/.test(locale):
    case /^zh\b/.test(locale):
      locale = "zh-cn";
      break;
    case /^de\b/.test(locale):
      locale = "de";
      break;
    case /^ro\b/.test(locale):
      locale = "ro";
      break;
    case /^ru\b/.test(locale):
      locale = "ru";
      break;
    case /^pl\b/.test(locale):
      locale = "pl";
      break;
    case /^ko\b/.test(locale):
      locale = "ko";
      break;
    case /^sk\b/.test(locale):
      locale = "sk";
      break;
    case /^tr\b/.test(locale):
      locale = "tr";
      break;
    case /^uk\b/.test(locale):
      locale = "uk";
      break;
    case /^vi\b/.test(locale):
      locale = "vi";
      break;
    case /^sv-se\b/.test(locale):
    case /^sv\b/.test(locale):
      locale = "sv-se";
      break;
    case /^nl-be\b/.test(locale):
      locale = "nl-be";
      break;
    case /^nl\b/.test(locale):
      locale = "nl";
      break;
    case /^nb\b/.test(locale):
    case /^no\b/.test(locale):
      locale = "no";
      break;

    case /^fa\b/.test(locale):
      locale = "fa";
      break;
    case /^ca\b/.test(locale):
      locale = "ca";
      break;
    default:
      locale = "en";
  }

  return locale;
}

export const rtlLanguages = ["he", "ar", "fa"];
export const i18n = createI18n({
  legacy: false,
  locale: "en" as string,
  fallbackLocale: "en",
  messages: { en: english } as Record<string, typeof english>,
});

export const isRtl = (locale?: string) =>
  rtlLanguages.includes(locale || i18n.global.locale.value);

const loaded = new Map<string, Promise<void>>();
let requestedLocale = "en";
export async function setLocale(locale: string) {
  if (locale !== "en" && !messageLoaders[`./${locale}.json`]) locale = "en";
  requestedLocale = locale;
  let pending = loaded.get(locale);
  if (!pending) {
    pending = Promise.all([
      locale === "en"
        ? Promise.resolve({ default: english })
        : messageLoaders[`./${locale}.json`](),
      dateLoaders[locale](),
    ]).then(([messages]) => {
      i18n.global.setLocaleMessage(locale, messages.default);
    });
    loaded.set(locale, pending);
  }
  try {
    await pending;
    if (requestedLocale !== locale) return;
    i18n.global.locale.value = locale;
    dayjs.locale(
      ({ "pt-pt": "pt", "sv-se": "sv", no: "nb" } as Record<string, string>)[
        locale
      ] || locale
    );
    setHtmlLocale(locale);
  } catch (error) {
    loaded.delete(locale);
    console.error("Could not load locale", locale, error);
  }
}
export function setHtmlLocale(locale: string) {
  const html = document.documentElement;
  html.lang = locale;
  if (isRtl(locale)) html.dir = "rtl";
  else html.dir = "ltr";
}

export default i18n;
