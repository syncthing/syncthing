angular.module('syncthing.core')
    .directive('identicon', ['$window', function ($window) {
        var svgNS = 'http://www.w3.org/2000/svg';

        function Identicon(value, size) {
            var svg = document.createElementNS(svgNS, 'svg');
            var shouldFillRectAt = function (row, col) {
                return !($window.parseInt(value.charCodeAt(row + col * size), 10) % 2);
            };
            var shouldMirrorRectAt = function (row, col) {
                return !(size % 2 && col === middleCol)
            };
            var mirrorColFor = function (col) {
                return size - col - 1;
            };
            var fillRectAt = function (row, col) {
                var rect = document.createElementNS(svgNS, 'rect');

                rect.setAttribute('x', (col * rectSize) + '%');
                rect.setAttribute('y', (row * rectSize) + '%');
                rect.setAttribute('width', rectSize + '%');
                rect.setAttribute('height', rectSize + '%');

                svg.appendChild(rect);
            };
            var row;
            var col;
            var middleCol;
            var rectSize;

            svg.setAttribute('class', 'identicon');
            size = size || 5;
            rectSize = 100 / size;
            middleCol = Math.ceil(size / 2) - 1;

            if (value) {
                value = value.toString().replace(/[\W_]/g, '');

                for (row = 0; row < size; ++row) {
                    for (col = middleCol; col > -1; --col) {
                        if (shouldFillRectAt(row, col)) {
                            fillRectAt(row, col);

                            if (shouldMirrorRectAt(row, col)) {
                                fillRectAt(row, mirrorColFor(col));
                            }
                        }
                    }
                }
            }

            return svg;
        }

        return {
            restrict: 'E',
            scope: {
                value: '='
            },
            link: function (scope, element, attributes) {
                element.append(new Identicon(scope.value));
            }
        }
    }]);
