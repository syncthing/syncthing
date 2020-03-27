// Adapted from https://www.chartjs.org/samples/latest/tooltips/custom-pie.html
export let tooltip: () => (tooltip: any) => void =
    function (): (tooltip: any) => void {
        return function (tooltip: any): void {
            // Tooltip Element
            const tooltipEl = document.getElementById('chartjs-tooltip');

            // Hide if no tooltip
            if (tooltip.opacity === 0) {
                tooltipEl.style.opacity = '0';
                return;
            }

            // Set caret Position
            tooltipEl.classList.remove('above', 'below', 'no-transform');
            if (tooltip.yAlign) {
                tooltipEl.classList.add(tooltip.yAlign);
            } else {
                tooltipEl.classList.add('no-transform');
            }

            function getBody(bodyItem) {
                return bodyItem.lines;
            }

            // Set Text
            if (tooltip.body) {
                let titleLines = tooltip.title || [];
                const bodyLines = tooltip.body.map(getBody);

                let innerHtml = '<thead>';

                titleLines.forEach(function (title) {
                    innerHtml += '<tr><th>' + title + '</th></tr>';
                });
                innerHtml += '</thead><tbody>';

                bodyLines.forEach(function (body, i) {
                    let colors = tooltip.labelColors[i];
                    let style = 'background:' + colors.backgroundColor;
                    style += '; border-color:' + colors.borderColor;
                    style += '; border-width: 2px';
                    let span = '<span class="chartjs-tooltip-key" style="' + style + '"></span>';
                    innerHtml += '<tr><td>' + span + body + '</td></tr>';
                });
                innerHtml += '</tbody>';

                let tableRoot = tooltipEl.querySelector('table');
                tableRoot.innerHTML = innerHtml;
            }

            var position = this._chart.canvas.getBoundingClientRect();

            // Display, position, and set styles for font
            tooltipEl.style.opacity = '1';
            tooltipEl.style.position = 'absolute';
            tooltipEl.style.left = position.left + window.pageXOffset + tooltip.caretX + 'px';
            tooltipEl.style.top = position.top + window.pageYOffset + tooltip.caretY + 'px';
            tooltipEl.style.padding = tooltip.yPadding + 'px ' + tooltip.xPadding + 'px';
            tooltipEl.style.pointerEvents = 'none';
        }
    };