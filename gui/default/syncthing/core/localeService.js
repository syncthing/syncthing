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
                        // Find the exact language in the list provided by the user's browser.
                        // Otherwise, find the first version of the language with a hyphenated
                        // suffix. That is, "en-US" sent by the browser will match "en-US" first,
                        // then "en", then "en-GB". Similarly, "en" will match "en" first, then
                        // "en-GB", then "en-US".

                        var i,
                            lang,
                            matching,
                            pattern = /-.*$/,
                            locale = _defaultLocale;

                        for (i = 0; i < langs.length; i++) {
                            lang = langs[i];

                            if (lang.length < 2) {
                                continue;
                            }

                            // The langs returned by the /rest/langs call will be in lower
                            // case. We compare to the lowercase version of the language
                            // code we have as well.

                            // Try to find the exact match first.
                            matching = _availableLocales.filter(function (possibleLang) {
                                possibleLang = possibleLang.toLowerCase();
                                if (possibleLang === lang) {
                                    return lang === possibleLang;
                                }
                            });

                            // Only look for a prefixed or suffixed match when no exact match exists.
                            if (!matching[0]) {
                                matching = _availableLocales.filter(function (possibleLang) {
                                    possibleLang = possibleLang.toLowerCase();
                                    lang = lang.replace(pattern, '');
                                    possibleLang = possibleLang.replace(pattern, '');
                                    return lang === possibleLang;
                                })
                            };

                            if (matching[0]) {
                                locale = matching[0];
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
