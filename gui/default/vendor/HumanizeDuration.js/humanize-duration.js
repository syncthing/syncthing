// HumanizeDuration.js - https://git.io/j0HgmQ

// @ts-check

/**
 * @typedef {string | ((unitCount: number) => string)} Unit
 */

/**
 * @typedef {("y" | "mo" | "w" | "d" | "h" | "m" | "s" | "ms")} UnitName
 */

/**
 * @typedef {Object} UnitMeasures
 * @prop {number} y
 * @prop {number} mo
 * @prop {number} w
 * @prop {number} d
 * @prop {number} h
 * @prop {number} m
 * @prop {number} s
 * @prop {number} ms
 */

/**
 * @internal
 * @typedef {[string, string, string, string, string, string, string, string, string, string]} DigitReplacements
 */

/**
 * @typedef {Object} Language
 * @prop {Unit} y
 * @prop {Unit} mo
 * @prop {Unit} w
 * @prop {Unit} d
 * @prop {Unit} h
 * @prop {Unit} m
 * @prop {Unit} s
 * @prop {Unit} ms
 * @prop {string} [decimal]
 * @prop {string} [delimiter]
 * @prop {DigitReplacements} [_digitReplacements]
 * @prop {boolean} [_numberFirst]
 * @prop {boolean} [_hideCountIf2]
 */

/**
 * @typedef {Object} Options
 * @prop {string} [language]
 * @prop {Record<string, Language>} [languages]
 * @prop {string[]} [fallbacks]
 * @prop {string} [delimiter]
 * @prop {string} [spacer]
 * @prop {boolean} [round]
 * @prop {number} [largest]
 * @prop {UnitName[]} [units]
 * @prop {string} [decimal]
 * @prop {string} [conjunction]
 * @prop {number} [maxDecimalPoints]
 * @prop {UnitMeasures} [unitMeasures]
 * @prop {boolean} [serialComma]
 * @prop {DigitReplacements} [digitReplacements]
 */

/**
 * @internal
 * @typedef {Required<Options>} NormalizedOptions
 */

(function () {
  // Fallback for `Object.assign` if relevant.
  var assign =
    Object.assign ||
    /** @param {...any} destination */
    function (destination) {
      var source;
      for (var i = 1; i < arguments.length; i++) {
        source = arguments[i];
        for (var prop in source) {
          if (has(source, prop)) {
            destination[prop] = source[prop];
          }
        }
      }
      return destination;
    };

  // Fallback for `Array.isArray` if relevant.
  var isArray =
    Array.isArray ||
    function (arg) {
      return Object.prototype.toString.call(arg) === "[object Array]";
    };

  // This has to be defined separately because of a bug: we want to alias
  // `gr` and `el` for backwards-compatiblity. In a breaking change, we can
  // remove `gr` entirely.
  // See https://github.com/EvanHahn/HumanizeDuration.js/issues/143 for more.
  var GREEK = language("έ", "μ", "ε", "η", "ώ", "λ", "δ", "χδ", ","); //

  /**
   * @internal
   * @type {Record<string, Language>}
   */
  var LANGUAGES = {
    // Afrikaans (Afrikaans)
    af: language("j", "mnd", "w", "d", "u", "m", "s", "ms", ","),
    // አማርኛ (Amharic)
    am: language("ዓ", "ወ", "ሳ", "ቀ", "ሰ", "ደ", "ሰከ", "ሳ", "ሚሊ"),
    //العربية (Arabic) (RTL)
    // https://github.com/EvanHahn/HumanizeDuration.js/issues/221#issuecomment-2119762498
    // year -> ع stands for "عام" or س stands for "سنة"
    // month -> ش stands for "شهر"
    // week -> أ stands for "أسبوع"
    // day -> ي stands for "يوم"
    // hour -> س stands for "ساعة"
    // minute -> د stands for "دقيقة"
    // second -> ث stands for "ثانية"
    ar: assign(language("س", "ش", "أ", "ي", "س", "د", "ث", "م ث", ","), {
      _hideCountIf2: true,
      _digitReplacements: ["۰", "١", "٢", "٣", "٤", "٥", "٦", "٧", "٨", "٩"]
    }),
    // български (Bulgarian)
    bg: language("г", "мес", "с", "д", "ч", "м", "сек", "мс", ","),
    // বাংলা (Bengali)
    bn: language("ব", "ম", "সপ্তা", "দ", "ঘ", "মি", "স", "মি.স"),
    // català (Catalan)
    ca: language("a", "mes", "set", "d", "h", "m", "s", "ms", ","),
    //کوردیی ناوەڕاست (Central Kurdish) (RTL)
    ckb: language("م چ", "چ", "خ", "ک", "ڕ", "ه", "م", "س", "."),
    // čeština (Czech)
    cs: language("r", "měs", "t", "d", "h", "m", "s", "ms", ","),
    // Cymraeg (Welsh)
    cy: language("b", "mis", "wth", "d", "awr", "mun", "eil", "ms"),
    // dansk (Danish)
    da: language("å", "md", "u", "d", "t", "m", "s", "ms", ","),
    // Deutsch (German)
    de: language("J", "mo", "w", "t", "std", "m", "s", "ms", ","),
    // Ελληνικά (Greek)
    el: GREEK,
    // English (English)
    en: language("y", "mo", "w", "d", "h", "m", "s", "ms"),
    // Esperanto (Esperanto)
    eo: language("j", "mo", "se", "t", "h", "m", "s", "ms", ","),
    // español (Spanish)
    es: language("a", "me", "se", "d", "h", "m", "s", "ms", ","),
    // eesti keel (Estonian)
    et: language("a", "k", "n", "p", "t", "m", "s", "ms", ","),
    // euskara (Basque)
    eu: language("u", "h", "a", "e", "o", "m", "s", "ms", ","),
    //فارسی (Farsi/Persian) (RTL)
    fa: language("س", "ما", "ه", "ر", "سا", "دقی", "ثانی", "میلی‌ثانیه"),
    // suomi (Finnish)
    fi: language("v", "kk", "vk", "pv", "t", "m", "s", "ms", ","),
    // føroyskt (Faroese)
    fo: language("á", "má", "v", "d", "t", "m", "s", "ms", ","),
    // français (French)
    fr: language("a", "m", "sem", "j", "h", "m", "s", "ms", ","),
    // Ελληνικά (Greek) (el)
    gr: GREEK,
    //עברית (Hebrew) (RTL)
    he: language("ש׳", "ח׳", "שב׳", "י׳", "שע׳", "ד׳", "שנ׳", "מל׳"),
    // hrvatski (Croatian)
    hr: language("g", "mj", "t", "d", "h", "m", "s", "ms", ","),
    // हिंदी (Hindi)
    hi: language("व", "म", "स", "द", "घ", "मि", "से", "मि.से"),
    // magyar (Hungarian)
    hu: language("é", "h", "hét", "n", "ó", "p", "mp", "ms", ","),
    // Indonesia (Indonesian)
    id: language("t", "b", "mgg", "h", "j", "m", "d", "md"),
    // íslenska (Icelandic)
    is: language("ár", "mán", "v", "d", "k", "m", "s", "ms"),
    // italiano (Italian)
    it: language("a", "me", "se", "g", "h", "m", "s", "ms", ","),
    // 日本語 (Japanese)
    ja: language("年", "月", "週", "日", "時", "分", "秒", "ミリ秒"),
    // ភាសាខ្មែរ (Khmer)
    km: language("ឆ", "ខ", "សប្តា", "ថ", "ម", "ន", "វ", "មវ"),
    // ಕನ್ನಡ (Kannada)
    kn: language("ವ", "ತ", "ವ", "ದ", "ಗಂ", "ನಿ", "ಸೆ", "ಮಿಸೆ"),
    // 한국어 (Korean)
    ko: language("년", "달", "주", "일", "시간", "분", "초", "밀리초"),
    // Kurdî (Kurdish)
    ku: language("sal", "m", "h", "r", "s", "d", "ç", "ms", ","),
    // ລາວ (Lao)
    lo: language("ປ", "ເດ", "ອ", "ວ", "ຊ", "ນທ", "ວິນ", "ມິລິວິນາທີ", ","),
    // lietuvių (Lithuanian)
    lt: language("met", "mėn", "sav", "d", "v", "m", "s", "ms", ","),
    // latviešu (Latvian)
    lv: language("g", "mēn", "n", "d", "st", "m", "s", "ms", ","),
    // македонски (Macedonian)
    mk: language("г", "мес", "н", "д", "ч", "м", "с", "мс", ","),
    // монгол (Mongolian)
    mn: language("ж", "с", "дх", "ө", "ц", "м", "с", "мс"),
    // मराठी (Marathi)
    mr: language("व", "म", "आ", "दि", "त", "मि", "से", "मि.से"),
    // Melayu (Malay)
    ms: language("thn", "bln", "mgg", "hr", "j", "m", "s", "ms"),
    // Nederlands (Dutch)
    nl: language("j", "mnd", "w", "d", "u", "m", "s", "ms", ","),
    // norsk (Norwegian)
    no: language("år", "mnd", "u", "d", "t", "m", "s", "ms", ","),
    // polski (Polish)
    pl: language("r", "mi", "t", "d", "g", "m", "s", "ms", ","),
    // português (Portuguese)
    pt: language("a", "mês", "sem", "d", "h", "m", "s", "ms", ","),
    // română (Romanian) săpt?
    ro: language("a", "l", "să", "z", "h", "m", "s", "ms", ","),
    // русский (Russian)
    ru: language("г", "мес", "н", "д", "ч", "м", "с", "мс", ","),
    // shqip (Albanian) orë? muaj?
    sq: language("v", "mu", "j", "d", "o", "m", "s", "ms", ","),
    // српски (Serbian)
    sr: language("г", "мес", "н", "д", "ч", "м", "с", "мс", ","),
    // தமிழ் (Tamil)
    ta: language("ஆ", "மா", "வ", "நா", "ம", "நி", "வி", "மி.வி"),
    // తెలుగు (Telugu)
    te: language("సం", "నె", "వ", "రో", "గం", "ని", "సె", "మి.సె"), //
    // українська (Ukrainian)
    uk: language("р", "м", "т", "д", "г", "хв", "с", "мс", ","),
    //اردو (Urdu) (RTL)
    ur: language("س", "م", "ہ", "د", "گ", "م", "س", "م س"),
    // slovenčina (Slovak)
    sk: language("r", "mes", "t", "d", "h", "m", "s", "ms", ","),
    // slovenščina (Slovenian)
    sl: language("l", "mes", "t", "d", "ur", "m", "s", "ms", ","),
    // svenska (Swedish)
    sv: language("å", "mån", "v", "d", "h", "m", "s", "ms", ","),
    // Kiswahili (Swahili)
    sw: assign(language("mw", "m", "w", "s", "h", "dk", "s", "ms"), {
      _numberFirst: true
    }),
    // Türkçe (Turkish)
    tr: language("y", "a", "h", "g", "sa", "d", "s", "ms", ","),
    // ไทย (Thai)
    th: language("ปี", "ด", "ส", "ว", "ชม", "น", "วิ", "มิลลิวินาที"),
    // o'zbek (Uzbek)
    uz: language("y", "o", "h", "k", "soa", "m", "s", "ms"),
    // Ўзбек (Кирилл) (Uzbek (Cyrillic))
    uz_CYR: language("й", "о", "х", "к", "соа", "д", "с", "мс"),
    // Tiếng Việt (Vietnamese)
    vi: language("n", "th", "t", "ng", "gi", "p", "g", "ms", ","),
    // 中文 (简体) (Chinese, simplified)
    zh_CN: language("年", "月", "周", "天", "时", "分", "秒", "毫秒"),
    // 中文 (繁體) (Chinese, traditional)
    zh_TW: language("年", "月", "週", "天", "時", "分", "秒", "毫秒")
  };

  /**
   * Helper function for creating language definitions.
   *
   * @internal
   * @param {Unit} y
   * @param {Unit} mo
   * @param {Unit} w
   * @param {Unit} d
   * @param {Unit} h
   * @param {Unit} m
   * @param {Unit} s
   * @param {Unit} ms
   * @param {string} [decimal]
   * @returns {Language}
   */
  function language(y, mo, w, d, h, m, s, ms, decimal) {
    /** @type {Language} */
    var result = { y: y, mo: mo, w: w, d: d, h: h, m: m, s: s, ms: ms };
    if (typeof decimal !== "undefined") {
      result.decimal = decimal;
    }
    return result;
  }

  /**
   * Helper function for Arabic.
   *
   * @internal
   * @param {number} c
   * @returns {0 | 1 | 2}
   */
  // function getArabicForm(c) {
  //   if (c === 2) {
  //     return 1;
  //   }
  //   if (c > 2 && c < 11) {
  //     return 2;
  //   }
  //   return 0;
  // }

  /**
   * Helper function for Polish.
   *
   * @internal
   * @param {number} c
   * @returns {0 | 1 | 2 | 3}
   */
  // function getPolishForm(c) {
  //   if (c === 1) {
  //     return 0;
  //   }
  //   if (Math.floor(c) !== c) {
  //     return 1;
  //   }
  //   if (c % 10 >= 2 && c % 10 <= 4 && !(c % 100 > 10 && c % 100 < 20)) {
  //     return 2;
  //   }
  //   return 3;
  // }

  /**
   * Helper function for Slavic languages.
   *
   * @internal
   * @param {number} c
   * @returns {0 | 1 | 2 | 3}
   */
  // function getSlavicForm(c) {
  //   if (Math.floor(c) !== c) {
  //     return 2;
  //   }
  //   if (
  //     (c % 100 >= 5 && c % 100 <= 20) ||
  //     (c % 10 >= 5 && c % 10 <= 9) ||
  //     c % 10 === 0
  //   ) {
  //     return 0;
  //   }
  //   if (c % 10 === 1) {
  //     return 1;
  //   }
  //   if (c > 1) {
  //     return 2;
  //   }
  //   return 0;
  // }

  /**
   * Helper function for Czech or Slovak.
   *
   * @internal
   * @param {number} c
   * @returns {0 | 1 | 2 | 3}
   */
  // function getCzechOrSlovakForm(c) {
  //   if (c === 1) {
  //     return 0;
  //   }
  //   if (Math.floor(c) !== c) {
  //     return 1;
  //   }
  //   if (c % 10 >= 2 && c % 10 <= 4 && c % 100 < 10) {
  //     return 2;
  //   }
  //   return 3;
  // }

  /**
   * Helper function for Lithuanian.
   *
   * @internal
   * @param {number} c
   * @returns {0 | 1 | 2}
   */
  // function getLithuanianForm(c) {
  //   if (c === 1 || (c % 10 === 1 && c % 100 > 20)) {
  //     return 0;
  //   }
  //   if (
  //     Math.floor(c) !== c ||
  //     (c % 10 >= 2 && c % 100 > 20) ||
  //     (c % 10 >= 2 && c % 100 < 10)
  //   ) {
  //     return 1;
  //   }
  //   return 2;
  // }

  /**
   * Helper function for Latvian.
   *
   * @internal
   * @param {number} c
   * @returns {boolean}
   */
  // function getLatvianForm(c) {
  //   return c % 10 === 1 && c % 100 !== 11;
  // }

  /**
   * @internal
   * @template T
   * @param {T} obj
   * @param {keyof T} key
   * @returns {boolean}
   */
  function has(obj, key) {
    return Object.prototype.hasOwnProperty.call(obj, key);
  }

  /**
   * @internal
   * @param {Pick<Required<Options>, "language" | "fallbacks" | "languages">} options
   * @throws {Error} Throws an error if language is not found.
   * @returns {Language}
   */
  function getLanguage(options) {
    var possibleLanguages = [options.language];

    if (has(options, "fallbacks")) {
      if (isArray(options.fallbacks) && options.fallbacks.length) {
        possibleLanguages = possibleLanguages.concat(options.fallbacks);
      } else {
        throw new Error("fallbacks must be an array with at least one element");
      }
    }

    for (var i = 0; i < possibleLanguages.length; i++) {
      var languageToTry = possibleLanguages[i];
      if (has(options.languages, languageToTry)) {
        return options.languages[languageToTry];
      }
      if (has(LANGUAGES, languageToTry)) {
        return LANGUAGES[languageToTry];
      }
    }

    throw new Error("No language found.");
  }

  /**
   * @internal
   * @param {Piece} piece
   * @param {Language} language
   * @param {Pick<Required<Options>, "decimal" | "spacer" | "maxDecimalPoints" | "digitReplacements">} options
   */
  function renderPiece(piece, language, options) {
    var unitName = piece.unitName;
    var unitCount = piece.unitCount;

    var spacer = options.spacer;
    var maxDecimalPoints = options.maxDecimalPoints;

    /** @type {string} */
    var decimal;
    if (has(options, "decimal")) {
      decimal = options.decimal;
    } else if (has(language, "decimal")) {
      decimal = language.decimal;
    } else {
      decimal = ".";
    }

    /** @type {undefined | DigitReplacements} */
    var digitReplacements;
    if ("digitReplacements" in options) {
      digitReplacements = options.digitReplacements;
    } else if ("_digitReplacements" in language) {
      digitReplacements = language._digitReplacements;
    }

    /** @type {string} */
    var formattedCount;
    var normalizedUnitCount =
      maxDecimalPoints === void 0
        ? unitCount
        : Math.floor(unitCount * Math.pow(10, maxDecimalPoints)) /
          Math.pow(10, maxDecimalPoints);
    var countStr = normalizedUnitCount.toString();

    if (language._hideCountIf2 && unitCount === 2) {
      formattedCount = "";
      spacer = "";
    } else {
      if (digitReplacements) {
        formattedCount = "";
        for (var i = 0; i < countStr.length; i++) {
          var char = countStr[i];
          if (char === ".") {
            formattedCount += decimal;
          } else {
            // @ts-ignore because `char` should always be 0-9 at this point.
            formattedCount += digitReplacements[char];
          }
        }
      } else {
        formattedCount = countStr.replace(".", decimal);
      }
    }

    var languageWord = language[unitName];
    var word;
    if (typeof languageWord === "function") {
      word = languageWord(unitCount);
    } else {
      word = languageWord;
    }

    if (language._numberFirst) {
      return word + spacer + formattedCount;
    }
    return formattedCount + spacer + word;
  }

  /**
   * @internal
   * @typedef {Object} Piece
   * @prop {UnitName} unitName
   * @prop {number} unitCount
   */

  /**
   * @internal
   * @param {number} ms
   * @param {Pick<Required<Options>, "units" | "unitMeasures" | "largest" | "round">} options
   * @returns {Piece[]}
   */
  function getPieces(ms, options) {
    /** @type {UnitName} */
    var unitName;

    /** @type {number} */
    var i;

    /** @type {number} */
    var unitCount;

    /** @type {number} */
    var msRemaining;

    var units = options.units;
    var unitMeasures = options.unitMeasures;
    var largest = "largest" in options ? options.largest : Infinity;

    if (!units.length) return [];

    // Get the counts for each unit. Doesn't round or truncate anything.
    // For example, might create an object like `{ y: 7, m: 6, w: 0, d: 5, h: 23.99 }`.
    /** @type {Partial<Record<UnitName, number>>} */
    var unitCounts = {};
    msRemaining = ms;
    for (i = 0; i < units.length; i++) {
      unitName = units[i];
      var unitMs = unitMeasures[unitName];

      var isLast = i === units.length - 1;
      unitCount = isLast
        ? msRemaining / unitMs
        : Math.floor(msRemaining / unitMs);
      unitCounts[unitName] = unitCount;

      msRemaining -= unitCount * unitMs;
    }

    if (options.round) {
      // Update counts based on the `largest` option.
      // For example, if `largest === 2` and `unitCount` is `{ y: 7, m: 6, w: 0, d: 5, h: 23.99 }`,
      // updates to something like `{ y: 7, m: 6.2 }`.
      var unitsRemainingBeforeRound = largest;
      for (i = 0; i < units.length; i++) {
        unitName = units[i];
        unitCount = unitCounts[unitName];

        if (unitCount === 0) continue;

        unitsRemainingBeforeRound--;

        // "Take" the rest of the units into this one.
        if (unitsRemainingBeforeRound === 0) {
          for (var j = i + 1; j < units.length; j++) {
            var smallerUnitName = units[j];
            var smallerUnitCount = unitCounts[smallerUnitName];
            unitCounts[unitName] +=
              (smallerUnitCount * unitMeasures[smallerUnitName]) /
              unitMeasures[unitName];
            unitCounts[smallerUnitName] = 0;
          }
          break;
        }
      }

      // Round the last piece (which should be the only non-integer).
      //
      // This can be a little tricky if the last piece "bubbles up" to a larger
      // unit. For example, "3 days, 23.99 hours" should be rounded to "4 days".
      // It can also require multiple passes. For example, "6 days, 23.99 hours"
      // should become "1 week".
      for (i = units.length - 1; i >= 0; i--) {
        unitName = units[i];
        unitCount = unitCounts[unitName];

        if (unitCount === 0) continue;

        var rounded = Math.round(unitCount);
        unitCounts[unitName] = rounded;

        if (i === 0) break;

        var previousUnitName = units[i - 1];
        var previousUnitMs = unitMeasures[previousUnitName];
        var amountOfPreviousUnit = Math.floor(
          (rounded * unitMeasures[unitName]) / previousUnitMs
        );
        if (amountOfPreviousUnit) {
          unitCounts[previousUnitName] += amountOfPreviousUnit;
          unitCounts[unitName] = 0;
        } else {
          break;
        }
      }
    }

    /** @type {Piece[]} */
    var result = [];
    for (i = 0; i < units.length && result.length < largest; i++) {
      unitName = units[i];
      unitCount = unitCounts[unitName];
      if (unitCount) {
        result.push({ unitName: unitName, unitCount: unitCount });
      }
    }
    return result;
  }

  /**
   * @internal
   * @param {Piece[]} pieces
   * @param {Pick<Required<Options>, "units" | "language" | "languages" | "fallbacks" | "delimiter" | "spacer" | "decimal" | "conjunction" | "maxDecimalPoints" | "serialComma" | "digitReplacements">} options
   * @returns {string}
   */
  function formatPieces(pieces, options) {
    var language = getLanguage(options);

    if (!pieces.length) {
      var units = options.units;
      var smallestUnitName = units[units.length - 1];
      return renderPiece(
        { unitName: smallestUnitName, unitCount: 0 },
        language,
        options
      );
    }

    var conjunction = options.conjunction;
    var serialComma = options.serialComma;

    var delimiter;
    if (has(options, "delimiter")) {
      delimiter = options.delimiter;
    } else if (has(language, "delimiter")) {
      delimiter = language.delimiter;
    } else {
      delimiter = " ";
    }

    /** @type {string[]} */
    var renderedPieces = [];
    for (var i = 0; i < pieces.length; i++) {
      renderedPieces.push(renderPiece(pieces[i], language, options));
    }

    if (!conjunction || pieces.length === 1) {
      return renderedPieces.join(delimiter);
    }

    if (pieces.length === 2) {
      return renderedPieces.join(conjunction);
    }

    return (
      renderedPieces.slice(0, -1).join(delimiter) +
      (serialComma ? "," : "") +
      conjunction +
      renderedPieces.slice(-1)
    );
  }

  /**
   * Create a humanizer, which lets you change the default options.
   *
   * @param {Options} [passedOptions]
   */
  function humanizer(passedOptions) {
    /**
     * @param {number} ms
     * @param {Options} [humanizerOptions]
     * @returns {string}
     */
    var result = function humanizer(ms, humanizerOptions) {
      // Make sure we have a positive number.
      //
      // Has the nice side-effect of converting things to numbers. For example,
      // converts `"123"` and `Number(123)` to `123`.
      ms = Math.abs(ms);

      var options = assign({}, result, humanizerOptions || {});

      var pieces = getPieces(ms, options);

      return formatPieces(pieces, options);
    };

    return assign(
      result,
      {
        language: "en",
        spacer: "",
        conjunction: "",
        serialComma: true,
        units: ["y", "mo", "w", "d", "h", "m", "s"],
        languages: {},
        round: false,
        unitMeasures: {
          y: 31557600000,
          mo: 2629800000,
          w: 604800000,
          d: 86400000,
          h: 3600000,
          m: 60000,
          s: 1000,
          ms: 1
        }
      },
      passedOptions
    );
  }

  /**
   * Humanize a duration.
   *
   * This is a wrapper around the default humanizer.
   */
  var humanizeDuration = assign(humanizer({}), {
    getSupportedLanguages: function getSupportedLanguages() {
      var result = [];
      for (var language in LANGUAGES) {
        if (has(LANGUAGES, language) && language !== "gr") {
          result.push(language);
        }
      }
      return result;
    },
    humanizer: humanizer
  });

  // @ts-ignore
  if (typeof define === "function" && define.amd) {
    // @ts-ignore
    define(function () {
      return humanizeDuration;
    });
  } else if (typeof module !== "undefined" && module.exports) {
    module.exports = humanizeDuration;
  } else {
    this.humanizeDuration = humanizeDuration;
  }
})();
