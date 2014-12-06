angular.module('syncthing.core')
    .provider('LocaleService', function () {

        var _defaultLocale,
            _availableLocales;

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
                // @TODO: check if there is nice way to utilize window.navigator.languages or similiar api.

                return $http.get(urlbase + "/lang");
            }

            function autoConfigLocale() {
                var params = $location.search();

                if(params.lang) {
                    $translate.use(params.lang);
                } else {
                    readBrowserLocales().success(function (langs) {
                        // Find the first language in the list provided by the user's browser
                        // that is a prefix of a language we have available. That is, "en"
                        // sent by the browser will match "en" or "en-US", while "zh-TW" will
                        // match only "zh-TW" and not "zh-CN".

                        var i,
                            lang,
                            matching,
                            locale = _defaultLocale;

                        for (i = 0; i < langs.length; i++) {
                            lang = langs[i];

                            if (lang.length < 2) {
                                continue;
                            }

                            matching = _availableLocales.filter(function (possibleLang) {
                                // The langs returned by the /rest/langs call will be in lower
                                // case. We compare to the lowercase version of the language
                                // code we have as well.
                                possibleLang = possibleLang.toLowerCase();
                                if (possibleLang.length > lang.length) {
                                    return possibleLang.indexOf(lang) === 0;
                                } else {
                                    return lang.indexOf(possibleLang) === 0;
                                }
                            });

                            if (matching.length >= 1) {
                                locale = matching[0];
                                break;
                            }
                        }
                        // Fallback if nothing matched
                        $translate.use(locale);
                    });
                }
            }

            function useLocale(language) {
                // @TODO: eventually check for valid locale format
                if (language) {
                    $translate.use(language);
                }
            }

            return {
                autoConfigLocale: autoConfigLocale,
                useLocale: useLocale
            }
        }];

    });
