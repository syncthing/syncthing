angular.module('syncthing.core')
    .filter('uncamel', function () {
        const reservedStrings = [
            'IDs', 'ID', // substrings must come AFTER longer keywords containing them
            'URL', 'UR',
            'API', 'QUIC', 'TCP', 'UDP', 'NAT', 'LAN', 'WAN',
            'KiB', 'MiB', 'GiB', 'TiB'
        ];
        return function (input) {
            if (!input || typeof input !== 'string') return '';
            const placeholders = {};
            let counter = 0;
            reservedStrings.forEach(word => {
                const placeholder = `__RSV${counter}__`;
                const re = new RegExp(word, 'g');
                input = input.replace(re, placeholder);
                placeholders[placeholder] = word;
                counter++;
            });
            input = input.replace(/([a-z0-9])([A-Z])/g, '$1 $2');
            Object.entries(placeholders).forEach(([ph, word]) => {
                input = input.replace(new RegExp(ph, 'g'), ` ${word} `);
            });
            let parts = input.split(' ');
            const lastPart = parts.pop();
            switch (lastPart) {
                case 'S': parts.push('(seconds)'); break;
                case 'M': parts.push('(minutes)'); break;
                case 'H': parts.push('(hours)'); break;
                case 'Ms': parts.push('(milliseconds)'); break;
                default: parts.push(lastPart); break;
            }
            parts = parts.map(part => {
                const match = reservedStrings.find(w => w.toUpperCase() === part.toUpperCase());
                return match || part.charAt(0).toUpperCase() + part.slice(1);
            });
            return parts.join(' ').replace(/\s+/g, ' ').trim();
        };
    });
