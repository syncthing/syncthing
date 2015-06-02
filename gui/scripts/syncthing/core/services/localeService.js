angular.module('syncthing.core')
    .provider('LocaleService', function () {
        'use strict';

        function detectLocalStorage() {
            // Feature detect localStorage; https://mathiasbynens.be/notes/localstorage-pattern
            try {
                var uid = new Date();
                var storage = window.localStorage;
                storage.setItem(uid, uid);
                var success = storage.getItem(uid) == uid;
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

        // native names of locales javascript escaped
        var _LOCALES_NAMES = { "af": "Afrikaans", "am": "\u12A0\u121B\u122D\u129B", "ar": "\u0627\u0644\u0639\u0631\u0628\u064A\u0629", "as": "\u0985\u09B8\u09AE\u09C0\u09AF\u09BC\u09BE", "ast": "Asturianu", "be": "\u0411\u0435\u043B\u0430\u0440\u0443\u0441\u043A\u0430\u044F", "bg": "\u0411\u044A\u043B\u0433\u0430\u0440\u0441\u043A\u0438", "bn": "\u09AC\u09BE\u0982\u09B2\u09BE", "bn-IN": "\u09AC\u09BE\u0982\u09B2\u09BE (\u09AD\u09BE\u09B0\u09A4)", "bo": "\u0F56\u0F7C\u0F51\u0F0B\u0F61\u0F72\u0F42", "br": "Brezhoneg", "brx": "\u092C\u094B\u0921\u094B", "bs": "Bosanski", "ca": "Catal\u00E0", "ca@valencia": "Catal\u00E0 (valenci\xE0)", "cs": "\u010De\u0161tina", "cy": "Welsh/Cymraeg", "da": "Dansk", "de": "Deutsch", "dgo": "\u0921\u094B\u0917\u0930\u0940", "dz": "\u0F62\u0FAB\u0F7C\u0F44\u0F0B\u0F41", "el": "\u0395\u03BB\u03BB\u03B7\u03BD\u03B9\u03BA\u03AC", "en-GB": "English (GB)", "en": "English", "en-ZA": "English (ZA)", "eo": "Esperanto", "es-ES": "Espa\u00F1ol (Espa\u00F1a)", "et": "Eesti keel", "eu": "Euskara", "fa": "\u0641\u0627\u0631\u0633\u0649", "fi": "Suomi", "fr": "Fran\xE7ais", "ga": "Gaeilge", "gd": "G\xE0idhlig", "gl": "Galego", "gu": "\u0A97\u0AC1\u0A9C\u0AB0\u0ABE\u0AA4\u0AC0", "he": "\u05E2\u05D1\u05E8\u05D9\u05EA", "hi": "\u0939\u093F\u0928\u094D\u0926\u0940", "hr": "Hrvatski", "hu": "Magyar", "id": "Bahasa Indonesia", "is": "\xCDslenska", "it": "Italiano", "ja": "\u65E5\u672C\u8A9E", "ka": "\u10E5\u10D0\u10E0\u10D7\u10E3\u10DA\u10D8", "kk": "\u049A\u0430\u0437\u0430\u049B\u0448\u0430", "km": "\u1781\u17D2\u1798\u17C2\u179A", "kmr-Latn": "Kurdish (latin script)", "kn": "\u0C95\u0CA8\u0CCD\u0CA8\u0CA1", "ko-KR": "\uD55C\uAD6D\uC5B4", "kok": "\u0915\u094B\u0902\u0915\u0923\u0940", "ks": "\uFEDA\uFEB8\uFEE4\uFEF3\uFEAE\uFEF3", "lb": "L\xEBtzebuergesch", "lo": "\u0E9E\u0EB2\u0EAA\u0EB2\u0EA5\u0EB2\u0EA7", "lt": "Lietuvi\u0173 kalba", "lv": "Latvie\u0161u", "mai": "\u092E\u0948\u0925\u093F\u0932\u0940", "mk": "\u043C\u0430\u043A\u0435\u0434\u043E\u043D\u0441\u043A\u0438", "ml": "\u0D2E\u0D32\u0D2F\u0D3E\u0D33\u0D02", "mn": "\u043C\u043E\u043D\u0433\u043E\u043B", "mni": "\u09AE\u09C8\u0987\u09A4\u09C8\u0987\u09B2\u09CB\u09A8", "mr": "\u092E\u0930\u093E\u0920\u0940", "my": "\u1019\u1014\u1039\u1019\u102C\u1005\u102C", "nb": "Bokm\u00E5l", "ne": "\u0928\u0947\u092A\u093E\u0932\u0940", "nl": "Nederlands", "nn": "Nynorsk", "nr": "Nd\xE9b\xE9l\xE9", "nso": "Sesotho sa Leboa", "oc": "Occitan", "om": "Afaan Oromo", "or": "\u0B13\u0B21\u0B3C\u0B3F\u0B06", "pa-IN": "\u0A2A\u0A70\u0A1C\u0A3E\u0A2C\u0A40", "pl": "Polski", "pt": "Portugu\xEAs", "pt-BR": "Portugu\xEAs (Brasil)", "pt-PT": "Portugu\xEAs (Portugal)", "ro_RO": "Rom\u00E2n\u0103 (Rom\u00E2nia)", "ru": "\u0420\u0443\u0441\u0441\u043A\u0438\u0439", "rw": "KinyaRwanda", "sa-IN": "\u0938\u0902\u0938\u094D\u0915\u0943\u0924\u092E\u094D", "sat": "\u0938\u0902\u0925\u093E\u0932\u0940", "sd": "\uFEB2\uFEE7\uFEA9\u06BE\u06CC", "si": "\u0DC3\u0DD2\u0D82\u0DC4\u0DBD", "sid": "Sidama", "sk": "Sloven\u010Dina", "sl": "Sloven\u0161\u010Dina", "sq": "Shqip", "sr": "\u0441\u0440\u043F\u0441\u043A\u0438", "sr-Latn": "Srpski latinicom", "ss": "SiSwati", "st": "Sesotho", "sv": "Svenska", "sw-TZ": "Kiswahili", "ta": "\u0BA4\u0BAE\u0BBF\u0BB4\u0BCD", "te": "\u0C24\u0C46\u0C32\u0C41\u0C17\u0C41", "tg": "\u0442\u043E\u04B7\u0438\u043A\u04E3", "th": "\u0E20\u0E32\u0E29\u0E32\u0E44\u0E17\u0E22", "tn": "Setswana", "tr": "T\xFCrk\xE7e", "ts": "Xitsonga", "tt": "\u0442\u0430\u0442\u0430\u0440 \u0442\u0435\u043B\u0435", "ug": "\uFE89\u06C7\uFEF2\uFECF\u06C7\uFEAD\u0686\u06D5", "uk": "\u0423\u043A\u0440\u0430\u0457\u043D\u0441\u044C\u043A\u0430", "uz": "\u045E\u0437\u0431\u0435\u043A", "ve": "Tshiven\u1E13a", "vi": "Ti\u1EBFng vi\u1EC7t", "xh": "IsiXhosa", "zh-CN": "\u4E2D\u6587 (\u7B80\u4F53)", "zh-TW": "\u4E2D\u6587 (\u6B63\u9AD4)", "zu": "IsiZulu" };

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

                if(params.lang) {
                    useLocale(params.lang, true);
                } else if (savedLang) {
                    useLocale(savedLang);
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
                        useLocale(locale);
                    });
                }
            }

            function useLocale(language, save2Storage) {
                if (language) {
                   $translate.use(language).then(function () {
                       if (save2Storage && _localStorage)
                            _localStorage[_SYNLANG] = language;
                    });
                }
            }

            return {
                autoConfigLocale: autoConfigLocale,
                useLocale: useLocale,
                getCurrentLocale: function() { return $translate.use() },
                getAvailableLocales: function() { return _availableLocales },
                getLocalesDisplayNames: function() { return _LOCALES_NAMES }
            }
        }];

    });
