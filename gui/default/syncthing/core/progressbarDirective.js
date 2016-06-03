angular.module('syncthing.core')
    .directive('progressbar', ['$window', function ($window) {
        var svgNS = 'http://www.w3.org/2000/svg';

        function Progressbar(value, size) {
            var svg = document.createElementNS(svgNS, 'svg');
            svg.setAttribute('viewBox', '0 0 100 100');

            var track = document.createElementNS(svgNS, 'circle');
            track.setAttribute('class', 'track');
            track.setAttribute('cx', '50');
            track.setAttribute('cy', '50');
            track.setAttribute('r', '37.5');
            track.setAttribute('stroke-width', '25');
            svg.appendChild(track);

            var bar = document.createElementNS(svgNS, 'circle');
            bar.setAttribute('class', 'bar');
            bar.setAttribute('cx', '50');
            bar.setAttribute('cy', '50');
            bar.setAttribute('r', '37.5');
            bar.setAttribute('stroke-width', '25');
            bar.setAttribute('stroke-dasharray',Math.ceil((37.5*2)*Math.PI));
            bar.setAttribute('transform', 'rotate(-90 50 50)');
            //bar.setAttribute('stroke-dashoffset',0);
            svg.appendChild(bar);
            return svg;
        }

        return {
            restrict: 'E',
            scope: {
                value: '='
            },
            controller: function($scope, $element, $attrs) {
                $element.append(new Progressbar($scope.value));
                $scope.$watch(function() {
                    $element.find('circle').attr('stroke-dashoffset', Math.ceil((((100-$scope.value)/100)*(37.5*2))*Math.PI));
                })
            }
        }
    }]);
