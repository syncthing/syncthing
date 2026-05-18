// Internationalization (i18n) module for the Svelte UI
// Mirrors the angular-translate setup from the original AngularJS UI

import { writable, derived, get } from 'svelte/store';

const SYN_LANG_KEY = 'SYN_LANG';

// Available locales and their display names
export const validLangs = ["ar","bg","ca","ca@valencia","cs","da","de","el","en","en-GB","eo","es","eu","fil","fr","fy","ga","gl","he-IL","hi","hr","hu","id","it","ja","ko-KR","lt","nl","pl","pt-BR","pt-PT","ro-RO","ru","sk","sl","sv","tr","uk","zh-CN","zh-HK","zh-TW"];

export const langPrettyprint = {"ar":"Arabic","bg":"Bulgarian","ca":"Catalan","ca@valencia":"Valencian","cs":"Czech","da":"Danish","de":"German","el":"Greek","en":"English","en-GB":"English (United Kingdom)","eo":"Esperanto","es":"Spanish","eu":"Basque","fil":"Filipino","fr":"French","fy":"Frisian","ga":"Irish","gl":"Galician","he-IL":"Hebrew (Israel)","hi":"Hindi","hr":"Croatian","hu":"Hungarian","id":"Indonesian","it":"Italian","ja":"Japanese","ko-KR":"Korean","lt":"Lithuanian","nl":"Dutch","pl":"Polish","pt-BR":"Portuguese (Brazil)","pt-PT":"Portuguese (Portugal)","ro-RO":"Romanian","ru":"Russian","sk":"Slovak","sl":"Slovenian","sv":"Swedish","tr":"Turkish","uk":"Ukrainian","zh-CN":"Chinese (Simplified Han script)","zh-HK":"Chinese (Traditional Han script, Hong Kong)","zh-TW":"Chinese (Traditional Han script)"};

// Store for current locale
export const currentLocale = writable('en');

// Store for loaded translations (keyed by locale code)
const translationCache = {};

// Store for the active translation map
export const translations = writable({});

// Loading state
export const i18nReady = writable(false);

/**
 * Load a language file from the server.
 * Language JSON files live at /assets/lang/lang-{code}.json
 */
async function loadLanguage(locale) {
  if (translationCache[locale]) {
    return translationCache[locale];
  }
  try {
    const resp = await fetch('/assets/lang/lang-' + locale + '.json');
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    const data = await resp.json();
    translationCache[locale] = data;
    return data;
  } catch (e) {
    console.error('Failed to load language file for', locale, e);
    return null;
  }
}

/**
 * Load English as fallback (always needed)
 */
let englishLoaded = false;
async function ensureEnglish() {
  if (!englishLoaded) {
    await loadLanguage('en');
    englishLoaded = true;
  }
}

/**
 * Switch to a given locale. Loads the language file if needed.
 * @param {string} locale - Locale code (e.g. 'de', 'en', 'zh-CN')
 * @param {boolean} save - Whether to persist to localStorage
 */
export async function useLocale(locale, save = false) {
  await ensureEnglish();
  const data = await loadLanguage(locale);
  if (!data && locale !== 'en') {
    // Fall back to English
    translations.set(translationCache['en'] || {});
    currentLocale.set('en');
    return;
  }
  // Merge with English fallback
  const en = translationCache['en'] || {};
  const merged = { ...en, ...(data || {}) };
  translations.set(merged);
  currentLocale.set(locale);
  document.documentElement.setAttribute('lang', locale);

  if (save) {
    try {
      window.localStorage[SYN_LANG_KEY] = locale;
    } catch (e) { /* ignore */ }
  }
  i18nReady.set(true);
}

/**
 * Auto-detect locale: check URL params, localStorage, then browser preferences via server API.
 */
export async function autoConfigLocale() {
  // Check URL params
  const params = new URLSearchParams(window.location.search);
  const urlLang = params.get('lang');
  if (urlLang) {
    await useLocale(urlLang, true);
    return;
  }

  // Check localStorage
  let savedLang;
  try {
    savedLang = window.localStorage[SYN_LANG_KEY];
  } catch (e) { /* ignore */ }

  if (savedLang) {
    await useLocale(savedLang);
    return;
  }

  // Ask the server for browser-preferred languages
  try {
    const resp = await fetch('/rest/svc/lang');
    if (resp.ok) {
      const langs = await resp.json();
      for (let i = 0; i < langs.length; i++) {
        const browserLang = langs[i];
        if (browserLang.length < 2) continue;

        const matching = validLangs.filter(function (possibleLang) {
          const pl = possibleLang.toLowerCase();
          if (pl.indexOf(browserLang) !== 0) return false;
          if (pl.length > browserLang.length) return pl[browserLang.length] === '-';
          return true;
        });

        if (matching.length >= 1) {
          await useLocale(matching[0]);
          return;
        }
      }
    }
  } catch (e) {
    // Server not available yet (e.g. not authenticated), fall through
  }

  // Default to English
  await useLocale('en');
}

/**
 * Translation function. Looks up the key in the current translation map.
 * Supports interpolation with {{variable}} syntax (matching angular-translate).
 * @param {string} key - The translation key (English text)
 * @param {Object} [params] - Optional interpolation parameters
 * @returns {string} Translated text
 */
export function t(key, params) {
  const trans = get(translations);
  let text = trans[key] || key;

  if (params) {
    for (const k in params) {
      // Replace both {{key}} and {%key%} patterns (angular-translate uses both)
      text = text.replace(new RegExp('\\{\\{' + k + '\\}\\}', 'g'), params[k]);
      text = text.replace(new RegExp('\\{%' + k + '%\\}', 'g'), params[k]);
    }
  }
  return text;
}

/**
 * Reactive translation function for use in Svelte components.
 * Returns a function that can be called to get the current translation.
 * Usage in components: const tl = $derived(tl); then use $translations and t()
 *
 * For simplicity, components should import { t } and { translations } and use:
 *   $translations; // subscribe to reactivity
 *   t('key')       // get translation
 *
 * Or use the reactive helper:
 *   import { tr } from '../lib/i18n.js';
 *   $: label = $tr('Some Label');
 */

/**
 * Get all available locale names for display in language selector.
 * Returns sorted array of { code, name } objects.
 */
export function getAvailableLocales() {
  return validLangs
    .filter(code => langPrettyprint[code])
    .map(code => ({ code, name: langPrettyprint[code] }))
    .sort((a, b) => a.name.localeCompare(b.name));
}
