angular.module('syncthing.core')
    .directive('languageSelect', function (LocaleService) {
        'use strict';
        return {
            restrict: 'EA',
            template:
                    '<a ng-if="visible" href="#" class="dropdown-toggle" data-toggle="dropdown" aria-expanded="true"><span class="fa fa-globe"></span>&nbsp;{{localesNames[currentLocale] || "English"}} <span class="caret"></span></a>'+
                    '<ul ng-if="visible" class="dropdown-menu">'+
                        '<li ng-repeat="(i,name) in localesNames" ng-class="{active: i==currentLocale}">'+
                            '<a href="#" data-ng-click="changeLanguage(i)">{{name}}</a>'+
                        '</li>'+
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
