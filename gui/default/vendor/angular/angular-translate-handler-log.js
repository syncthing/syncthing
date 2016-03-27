/*!
 * angular-translate - v2.11.0 - 2016-03-20
 *
 * Copyright (c) 2016 The angular-translate team, Pascal Precht; Licensed MIT
 */
(function (root, factory) {
  if (typeof define === 'function' && define.amd) {
    // AMD. Register as an anonymous module unless amdModuleId is set
    define([], function () {
      return (factory());
    });
  } else if (typeof exports === 'object') {
    // Node. Does not work with strict CommonJS, but
    // only CommonJS-like environments that support module.exports,
    // like Node.
    module.exports = factory();
  } else {
    factory();
  }
}(this, function () {

$translateMissingTranslationHandlerLog.$inject = ['$log'];
angular.module('pascalprecht.translate')

/**
 * @ngdoc object
 * @name pascalprecht.translate.$translateMissingTranslationHandlerLog
 * @requires $log
 *
 * @description
 * Uses angular's `$log` service to give a warning when trying to translate a
 * translation id which doesn't exist.
 *
 * @returns {function} Handler function
 */
.factory('$translateMissingTranslationHandlerLog', $translateMissingTranslationHandlerLog);

function $translateMissingTranslationHandlerLog ($log) {

  'use strict';

  return function (translationId) {
    $log.warn('Translation for ' + translationId + ' doesn\'t exist');
  };
}

$translateMissingTranslationHandlerLog.displayName = '$translateMissingTranslationHandlerLog';
return 'pascalprecht.translate';

}));
