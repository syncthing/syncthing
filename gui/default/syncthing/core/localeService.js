angular.module('syncthing.core')
    .provider('LocaleService', function () {
        'use strict';

        function detectLocalStorage() {
            // Feature detect localStorage; https://mathiasbynens.be/notes/localstorage-pattern
            try {
                var uid = new Date();
                var storage = window.localStorage;
                storage.setItem(uid, uid);
                storage.removeItem(uid);
                return storage;
            } catch (exception) {
                return undefined;
            }
        }

        var _defaultLocale,
            _availableLocales,
            _localStorage = detectLocalStorage();

        var _SYNLANG = "SYN_LANG"; // const key for localStorage

        this.setDefaultLocale = function (locale) {
            _defaultLocale = locale;
        };

        this.setAvailableLocales = function (locales) {
            _availableLocales = locales;
        };


        this.$get = ['$http', '$translate', '$location', function ($http, $translate, $location) {

            /**
             * Requests the server in order to get the browser's requested locale strings.
             *
             * @returns promise which on success resolves with a locales array
             */
            function readBrowserLocales() {
                // @TODO: check if there is nice way to utilize window.navigator.languages or similar api.

                return $http.get(urlbase + "/svc/lang");
            }

            function autoConfigLocale() {
                var params = $location.search();
                var savedLang;
                if (_localStorage) {
                    savedLang = _localStorage[_SYNLANG];
                }

                if (params.lang) {
                    useLocale(params.lang, true);
                } else if (savedLang) {
                    useLocale(savedLang);
                } else {
                    readBrowserLocales().success(function (langs) {
                        // Find the first language in the list provided by the user's browser
                        // that is a prefix of a language we have available. That is, "en"
                        // sent by the browser will match "en" or "en-US", while "zh-TW" will
                        // match only "zh-TW" and not "zh" or "zh-CN".

                        var i,
                            browserLang,
                            possibleLang,
                            matchingLang,
                            pattern = /-.*$/,
                            locale = _defaultLocale;

                        for (i = 0; i < langs.length; i++) {
                            browserLang = langs[i];
                            if (browserLang.length < 2) {
                                continue;
                            }

                            _availableLocales.reverse().filter(function (guiLang) {
                                // The langs returned by the /rest/langs call will be in lower
                                // case. We compare to the lowercase version of the language
                                // code we have as well. If no exact match found, the last match
                                // wins, so we need reverse the order to make "zh" match with
                                // "zh-CN" and not "zh-TW".
                                possibleLang = guiLang.toLowerCase();
                                if (possibleLang === browserLang) {
                                    // Skip further checking if exact match found.
                                    return matchingLang = guiLang;
                                } else if (possibleLang.length > browserLang.length) {
                                    // Match "en-GB" with "en" but not "en" with "en-GB". Keep
                                    // checking even if found already, as exact match may still
                                    // be found later.
                                    possibleLang = possibleLang.replace(pattern, '');
                                    if (possibleLang === browserLang) {
                                        matchingLang = guiLang;
                                    }
                                }
                            });

                            if (matchingLang) {
                                locale = matchingLang;
                                break;
                            }
                        }
                        // Fallback if nothing matched
                        useLocale(locale);
                    });
                }
            }

            function useLocale(language, save2Storage) {
                if (language) {
                    $translate.use(language).then(function () {
                        document.documentElement.setAttribute("lang", language);
                        if (save2Storage && _localStorage)
                            _localStorage[_SYNLANG] = language;
                    });
                }
            }

            return {
                autoConfigLocale: autoConfigLocale,
                useLocale: useLocale,
                getCurrentLocale: function () { return $translate.use() },
                getAvailableLocales: function () { return _availableLocales },
                // langPrettyprint comes from an included global
                getLocalesDisplayNames: function () { return langPrettyprint }
            }
        }];

    });
