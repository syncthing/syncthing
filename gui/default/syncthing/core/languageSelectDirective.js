angular.module('syncthing.core')
    .directive('languageSelect', function (LocaleService) {
        'use strict';
        return {
            restrict: 'EA',
            template:
                '<a ng-if="visible" href="#" class="dropdown-toggle" data-toggle="dropdown" aria-expanded="false"><span class="fas fa-globe"></span><span class="hidden-xs">&nbsp;{{localesNames[currentLocale] || "English"}}</span> <span class="caret"></span></a>' +
                '<ul ng-if="visible" class="dropdown-menu">' +
                '<li ng-repeat="name in localesNamesInvKeys" ng-class="{active: localesNamesInv[name]==currentLocale}">' +
                '<a href="#" data-ng-click="changeLanguage(localesNamesInv[name])">{{name}}</a>' +
                '</li>' +
                '</ul>',

            link: function ($scope) {
                var availableLocales = LocaleService.getAvailableLocales();
                var localeNames = LocaleService.getLocalesDisplayNames();
                var availableLocaleNames = {};

                // get only locale names that present in available locales
                for (var i = 0; i < availableLocales.length; i++) {
                    var a = availableLocales[i];
                    if (localeNames[a]) {
                        availableLocaleNames[a] = localeNames[a];
                    } else {
                        // show code lang if it is not in the dict
                        availableLocaleNames[a] = '[' + a + ']';
                    }
                }
                $scope.localesNames = availableLocaleNames;

                var invert = function (obj) {
                    var new_obj = {};

                    for (var prop in obj) {
                        if (obj.hasOwnProperty(prop)) {
                            new_obj[obj[prop]] = prop;
                        }
                    }
                    return new_obj;
                };
                $scope.localesNamesInv = invert($scope.localesNames);
                $scope.localesNamesInvKeys = Object.keys($scope.localesNamesInv).sort();

                $scope.visible = $scope.localesNames && $scope.localesNames['en'];

                // using $watch cause LocaleService.currentLocale will be change after receive async query accepted-languages
                // in LocaleService.readBrowserLocales
                var remove_watch = $scope.$watch(LocaleService.getCurrentLocale, function (newValue) {
                    if (newValue) {
                        $scope.currentLocale = newValue;
                        remove_watch();
                    }
                });

                $scope.changeLanguage = function (locale) {
                    LocaleService.useLocale(locale, true);
                    $scope.currentLocale = locale;
                };
            }
        };
    });
