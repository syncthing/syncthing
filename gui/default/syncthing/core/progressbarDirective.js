angular.module('syncthing.core')
    .directive('progressbar', ['$window', function ($window) {
        var svgNS = 'http://www.w3.org/2000/svg';

        function Progressbar(value, size) {
            var thickness = 1;										// 1 meaning same as radius, 0.1 meaning 10% of radius.

            var svg = document.createElementNS(svgNS, 'svg');
            svg.setAttribute('viewBox', '0 0 100 100');
            
            var maxradius = 50;
            var sw = Math.floor(maxradius*thickness);
            var r = Math.floor(maxradius-(sw/2));
            var track = document.createElementNS(svgNS, 'circle');
            track.setAttribute('class', 'track');
            track.setAttribute('cx', '50');
            track.setAttribute('cy', '50');
            track.setAttribute('r', r);
            track.setAttribute('stroke-width', sw);
            svg.appendChild(track);

            var bar = document.createElementNS(svgNS, 'circle');
            bar.setAttribute('class', 'bar');
            bar.setAttribute('cx', '50');
            bar.setAttribute('cy', '50');
            bar.setAttribute('r', r);
            bar.setAttribute('stroke-width', sw);
            bar.setAttribute('stroke-dasharray', Math.ceil((r*2)*Math.PI));
            bar.setAttribute('transform', 'rotate(-90 50 50)');			// fix orientation
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
                    $element.find('.bar').attr('stroke-dashoffset', Math.ceil((((100-$scope.value)/100)*($element.find('.bar').attr('r')*2))*Math.PI));
                })
            }
        }
    }]);
