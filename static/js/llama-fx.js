(function () {
    var stage = document.querySelector('.llama-stage');
    if (!stage) return;

    var fx = document.getElementById('llama-fx');
    var chart = document.getElementById('chart');
    var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    var colors = ['#9F8CD7', '#80BCCF', '#D883CE', '#7CCDCA', '#E63946'];

    function heart(x, y, s, fill) {
        return '<path transform="translate(' + x + ' ' + y + ') scale(' + s + ')" d="M0 6 C-12 -4 -6 -12 0 -5 C6 -12 12 -4 0 6 Z" fill="' + fill + '"/>';
    }

    function star(x, y, s) {
        return '<path transform="translate(' + x + ' ' + y + ') scale(' + s + ')" d="M0 -4 L1.2 -1.2 L4 -1.2 L1.8 0.6 L2.6 3.6 L0 1.8 L-2.6 3.6 L-1.8 0.6 L-4 -1.2 L-1.2 -1.2 Z" fill="#FAFAFA"/>';
    }

    function burst(cx, cy, r1, r2, color) {
        var out = '';
        for (var i = 0; i < 8; i++) {
            var a = i * Math.PI / 4;
            out += '<line x1="' + (cx + Math.cos(a) * r1).toFixed(1) + '" y1="' + (cy + Math.sin(a) * r1).toFixed(1) +
                '" x2="' + (cx + Math.cos(a) * r2).toFixed(1) + '" y2="' + (cy + Math.sin(a) * r2).toFixed(1) +
                '" stroke="' + color + '" stroke-width="2.5" stroke-linecap="round"/>';
        }
        return out + '<circle cx="' + cx + '" cy="' + cy + '" r="2" fill="' + color + '"/>';
    }

    var HATS = {
        santa: {
            svg: '<svg viewBox="0 0 100 100"><path d="M14 100 Q40 96 50 60 Q56 30 90 8 Q78 40 86 100 Z" fill="#E63946"/><rect x="8" y="90" width="84" height="12" rx="6" fill="#FAFAFA"/><circle cx="90" cy="8" r="8" fill="#FAFAFA"/></svg>'
        },
        party: {
            svg: '<svg viewBox="0 0 100 100"><polygon points="50,0 20,100 80,100" fill="#D883CE"/><line x1="36" y1="60" x2="64" y2="60" stroke="#9F8CD7" stroke-width="6"/><line x1="28" y1="85" x2="72" y2="85" stroke="#80BCCF" stroke-width="6"/><circle cx="50" cy="4" r="7" fill="#7CCDCA"/></svg>'
        },
        bunny: {
            svg: '<svg viewBox="0 0 100 100"><ellipse cx="32" cy="48" rx="11" ry="46" fill="#FAFAFA"/><ellipse cx="68" cy="48" rx="11" ry="46" fill="#FAFAFA"/><ellipse cx="32" cy="50" rx="5" ry="36" fill="#D883CE"/><ellipse cx="68" cy="50" rx="5" ry="36" fill="#D883CE"/></svg>'
        },
        pilgrim: {
            svg: '<svg viewBox="0 0 100 100"><rect x="24" y="30" width="52" height="60" fill="#1a1a1a"/><rect x="6" y="84" width="88" height="14" rx="7" fill="#1a1a1a"/><rect x="40" y="56" width="20" height="14" fill="#FAFAFA"/></svg>'
        },
        witch: {
            svg: '<svg viewBox="0 0 100 100"><polygon points="50,0 22,84 78,84" fill="#1a1a1a"/><rect x="14" y="80" width="72" height="12" rx="6" fill="#1a1a1a"/><rect x="24" y="66" width="52" height="7" fill="#D883CE"/></svg>'
        },
        bandana: {
            svg: '<svg viewBox="0 0 100 100">' +
                burst(50, 20, 5, 15, '#E63946') +
                burst(22, 40, 4, 11, '#FAFAFA') +
                burst(80, 36, 4, 12, '#80BCCF') +
                '<rect x="28" y="52" width="44" height="38" fill="#2B4C9B"/>' +
                '<rect x="28" y="74" width="44" height="8" fill="#E63946"/>' +
                '<rect x="14" y="88" width="72" height="10" rx="5" fill="#2B4C9B"/>' +
                star(40, 63, 1.1) + star(50, 60, 1.1) + star(60, 63, 1.1) +
                '</svg>'
        },
        bow: {
            svg: '<svg viewBox="0 0 100 100">' +
                heart(74, 26, 1.1, '#D883CE') +
                heart(26, 32, 0.8, '#E63946') +
                '<path d="M16 92 Q16 40 50 40 Q84 40 84 92 Z" fill="#E63946"/>' +
                '<rect x="8" y="88" width="84" height="10" rx="5" fill="#D883CE"/>' +
                heart(36, 72, 1.1, '#FAFAFA') + heart(62, 66, 1.3, '#FAFAFA') + heart(48, 52, 0.8, '#FAFAFA') +
                '</svg>'
        },
        tophat: {
            svg: '<svg viewBox="0 0 100 100"><rect x="24" y="22" width="52" height="66" fill="#7CCDCA"/><rect x="24" y="72" width="52" height="8" fill="#1a1a1a"/><rect x="8" y="86" width="84" height="12" rx="6" fill="#7CCDCA"/></svg>'
        }
    };

    var LLAMA_PATH = "M716 503.2c-.6 3.9.5 8.1.5 8.1-5.5-1.5-6.6-5-6.6-5-.5 4.5-6 11.1-6 11.1v-8.6c-4.8 4.5-1.7 13.1-1.7 13.1-4-1-5.6-4-5.6-4-4 5 1.2 11.1 1.2 11.1-4.5-.5-7-9.6-6.5-7.1s-1 11.1-1 11.1c.5-3.5-3-7.1-3-7.1 1 4-3.5 29.2-3.5 29.2l-2-7.6c.5 5-2 25.7-2 25.7l-2-3.5c2.5 8.6-3 29.2-3 27.2s-2.1-9.6-2.1-9.6c0 8.1-4.4 32.7-4.4 32.7 2.5 7.1-5 27.7-5 26.2s-1-5-1-5c-.5 7.6-4.5 19.6-4.5 19.6 1-6-2.5-2-2.5-2 .5 11.6-11.6 33.2-11.6 33.2.5-5-3.5 2-3.5 2-4 6.5-4.5 16.6-4.5 16.6l-2-2c-1.5 2 0 14.1-1 12.6s-3-.5-3-.5c-1.5 3-.5 11.1-.5 11.1l-2-4.5 1 11.6c-3-5-1 4.5-1 4.5 0 4.5 9.1 24.2 9.1 24.2 3.5-1.5 14.6.5 14.6.5 8.1 2.5 12.4 15.1 12.4 15.1s-7.6-4.5-5.5-3.5 2.3 6.9 2.3 6.9c-10.1 4.4-38.8-2.5-38.8-2.5-10.8-1.9-8.9-21.6-8.9-21.6l-1.9-5.7c-.6-8.9-2.5-5.7-2.5-5.7-3.8-7.6-5.2-14.1-5.2-14.1v4.5c-6-7.6-9.2-19.7-8.5-17.1s-1.2 5.7-1.2 10.8-2.4 20.3-2.4 20.3l-1.8-4.4-2.5 11-3.1-2.7c-3.2 7.6 7.6 29.2 7.6 29.2 5.8-1.9 13.3.6 13.3.6 8.2 3.2 13.3 17.1 13.3 17.1s-7-3.8-5.7-1.3c1.3 2.5 1.3 7 1.3 7-6.3 2.5-38.7-4.4-38.7-4.4-13.3-8.2-11.4-17.8-11.4-20.3s-3.2-21.6-3.2-21.6c-2.5-1.9-3.8 7.6-3.8 7.6-1.3-4.4 0-14.6 0-14.6l-2.5 6.3c-1.3-5.1 0-19 0-19-3.2-10.8-4.4 0-4.4 0-14.6-14.6-16.5-52-16.5-52-1.9-.6-1.9 6.3-1.9 6.3-6.3-8.2.6-40 .6-40l-3.8 8.9c-7.6-9.5-4.4-45-4.4-45l-2.5 4.4c-5.1-12.7 1.3-46.3 1.3-46.3l-3.8 3.2c7.6-29.8-2-46.3-2-46.3l-6.6 7.7s3.3-3.7.5-7c0 0-6.1 6.6-11.8 7.1 0 0 5.7-4.2 4.7-5.6s-9.9-2.3-11.8 3.3l-1.4-3.3s-1.4 6.6-.5 8.5l-4.2-8s-4.2 6.1-19.8 5.7c0 0 6.6-1.4 7.5-3.3l.9-1.9s-25.5 3.3-28.3 9.4l2.2-6.1-7.6 5.5v-3.6c-1.2-1.9-3.4 1.9-3.4 1.9-1.4-.5-8-4.2-8-4.2-3.8-.9-.3 3.8-.3 3.8-4.7-2.4-13.1-5.7-13.1-5.7-1.9 0 1 4.7 1 4.7-5.2-1.9-22.1-10.4-22.1-10.4-1.9 0 1.9 5.2 1.9 5.2-4.7-.9-12.7-7.5-12.7-7.5l-1.9 2.4c-3.8-4.7-9.4-7.1-11.3-7.1-1.9 0 1.9 5.2 1.9 5.2-10.4-4.7-20.2-16-19.3-14.1.9 1.9 2.4 6.2 2.4 6.2-11.8-14.1-19.7-8.9-19.7-8.9s-12.6 40.7-24.7 54.6c0 0 4.4-8.2 1.9-10.8s-5.7 24.7-12.7 31.7l2.5-8.9s-7 15.9-11.4 19l.6-7S278 625 274.4 625.6v-6.8s-10.8 10.6-11.4 23.9c0 0-4.8 37.4-1.6 44.4l-2.4-4.4s3.2 15.9 1.3 22.8l-2.5-5.7s3.8 19.7 1.9 26.6l-1.9-6.3s-3.8 12 3.2 22.8c0 0 23.5 25.4 25.4 30.4l15.2 3.2s11.4 7 15.2 14l-8.9 1.9 4.4 5.1s-20.9 8.9-35.5-1.3c0 0-17.1-1.6-17.1-12.7 0 0-8.2-12.1-12.1-16.5 0 0-18.4-18.4-14.6-29.2 0 0-7-14.6-7-8.2 0 0-7-12.7-7-5.1 0 0-8.2-5.1-7.6-16.5 0 0-1.9 7-.6 11.4 0 0-8.2-10.8-8.2-17.1 0 0-4.4 11.4-1.3 21.6 0 0 12.7 17.8 15.9 21.6l7.6 1.3s9.8 5.8 13.3 15.9c0 0-14.6 8.8-34.9 1.8 0 0-9.3-.9-8.7-10.6 0 0-5.8-9.6-12.2-16.6 0 0-9.5-10.1-7-23.5 0 0 3.2-4.4-3.2-10.8 0 0-3.1-3.8-3.1 1.3 0 0-9.5-10.1-9.5-14v7s-7-14-7-20.3l-3.5 4.4s.2-24.7 2.7-29.8l-5 9.5s0-26 3.2-29.8l-3.2 3.8s-4.4-31.1 1.3-35.5c0 0-8.2 16.5-8.2 9.5 0 0 1.3-32.3 11.4-43.1 0 0-3.8-.6-5.7 2.5 0 0 8.2-19.7 16.5-25.4l-6.3 1.9s16.5-20.9 25.4-25.4c0 0 7.6-9.5 7.6-20.3l-3.2-5.1-3.3 3.3c3.3-3.3 1.4-17.4 1.4-17.4 0 4.2-3.8 10.4-3.8 10.4 3.8-3.8-6.6-29.7-6.6-29.7-1.4.9 2.8 15.1 2.8 15.1-10.4-22.6-10.7-43.8-10.7-43.8-.5-25-7.7-25.5-7.7-25.5-2.4.5-.9 8-2.8 7.1-1.9-.9-1.9-4.7-1.9-4.7-4.7.5.5 9.4.5 9.4-30.2-15.1-16-80.6-18.4-79.7-2.4.9-3.8 12.3-3.8 12.3-1.4-22.2 11.3-45.2 11.3-45.2l-6.6 4.2c5.2-5.2 14.1-28.8 14.1-28.8l-4.2 3.3c5.2-15.1 26.9-23.6 26.9-23.6 4.2 0 15.6-15.1 15.6-15.1l-8.5 5.3C204 223.9 280.6 204 280.6 204h-6c28.3-14.5 91.4-3.1 91.4-3.1l-5.2-2.8c7.5-1.4 32.1 9.3 32.1 9.3-1.4 0-4.7-4.3-4.7-4.3 14.1 8 58 15 58 15 44.8 3.8 50.9 14.1 50.9 14.1l-4.2-4.7c6.6 5.7 25.5 1.9 25.5 1.9l-7.1-1.4c17.9-2.8 60.8 8 60.8 8l-4.7-2.8c8-1.4 36.8 9.8 36.8 9.8 21.7 9.4 27.8-7 27.8-7-3.8 5.7-9.4 1.9-9.4 1.9 8 .9 15.6-22.6 15.6-22.6l-2.8 2.8c2.8-13.7 11.3-25.9 11.3-25.9l-5.2 2.8c6.6-8 19.3-45.7 19.3-45.7-3.3.9-4.2 5.7-4.2 5.7 4.2-19.3 17.4-41.5 17.4-41.5l-4.2 4.7c7.1-22.6 25.9-41.5 24-39.1-1.9 2.4-5.7 1.4-5.7 1.4 13.7-9 14.6-17.4 14.6-17.4.5-1.3 1.2-2.6 2-4.1 7.2-11.7 26.7-27 26.7-27l-4.7 1.4c11.8-14.1 31.4-20.7 31.4-20.7S758 .1 776.4-12.5c0 0 27.9-30.4 33.6-25.3 0 0-28.6 38.7-21.6 50.1 0 0 7 1.3 10.8 3.2 0 0 1.3-13.9 17.1-22.2 0 0 22.8-26.6 31.7-23.5 0 0-33 41.9-26.6 49.5 0 0 23.5.6 28.5 12.1 0 0-8.2-8.2-17.8-3.2 0 0 22.2 3.8 17.8 22.8 0 0-3.8-5.1-7.6-5.1 0 0 7 9.5 6.3 17.8 0 0-5.7-3.2-9.5-1.9 0 0 12.7 11.4 19.7 15.2 0 0 24.4 7.8 26.3 35.1 0 0 10.8 18.9-4.2 20.7 0 0-6.1 11.3-14.6 7.5 0 0-13.2-.5-20.3-9 0 0-11.3-3.1-19.1 2.5l4 .9-8.4 1.6 3.4.6s-10.6 2.8-11.2 1.6l5.3.9s-8.4 3.4-17.1 3.7c0 0-8.4-2.5-7.8 11.8l-1.6-4.4s-2 7.8-.1 16.8c0 0-2.6-4-2.6-5.6v-2s-4.1 18.3.6 35.7c0 0 3 15 2 24.2 0 0-.5-3.4-1.9-6.5 0 0 1.9 18.2-3.7 35.8 0 0-2.2 11.6 1.5 19.5l-3-2.6s-1.1 9.8 2.3 16.5L786 280s4 22.1-.2 32.3c0 0 0-3.8-1.1-6.8 0 0-.2 8.3 1.3 13.1l-2.2-1.1s-3 9.8.4 16.5l-2.6-2.6s2.3 7.5-.4 14.6l-1.2-3s-3.9 12.8-7.7 18.9c0 0-5.2 7.5-1.4 18.9l-3.8-3.3s-2.8 12.7 2.4 17.9l-5.2-5.2s.9 13.7-.5 18.9l-1.9-8s-2.8 9.9 1 19.8c0 0-4.7-4.7-4.7-6.1 0-1.4-1.8 14.7 1.9 19.4 0 0-4.6-1.7-5.1-5.5 0 0-6.9 14-1.9 26.1l-4-4s-12.1 12.6-8.1 23.7c0 0-3.5-4.5-3-6s-2 18.1-9.1 25.7c0 0 3.5-6.6 0-10.1 0 0-4.5 20.7-7.6 23.2.2 0-4.3-10.1-5.3-4.1z";

    function pick(list) {
        return list[Math.floor(Math.random() * list.length)];
    }

    function removeLater(el, ms) {
        setTimeout(function () { el.remove(); }, ms);
    }

    function easterDate(year) {
        var a = year % 19, b = Math.floor(year / 100), c = year % 100;
        var d = Math.floor(b / 4), e = b % 4, f = Math.floor((b + 8) / 25);
        var g = Math.floor((b - f + 1) / 3), h = (19 * a + b - d - g + 15) % 30;
        var i = Math.floor(c / 4), k = c % 4, l = (32 + 2 * e + 2 * i - h - k) % 7;
        var m = Math.floor((a + 11 * h + 22 * l) / 451);
        var month = Math.floor((h + l - 7 * m + 114) / 31);
        var day = ((h + l - 7 * m + 114) % 31) + 1;
        return { month: month, day: day };
    }

    function thanksgivingDay(year) {
        var firstWeekday = new Date(year, 10, 1).getDay();
        return 1 + ((4 - firstWeekday + 7) % 7) + 21;
    }

    var HOLIDAY_WINDOW_DAYS = 5;

    function holidaysFor(y) {
        var easter = easterDate(y);
        return [
            { hat: 'party', date: new Date(y, 0, 1) },
            { hat: 'bow', date: new Date(y, 1, 14) },
            { hat: 'tophat', date: new Date(y, 2, 17) },
            { hat: 'bunny', date: new Date(y, easter.month - 1, easter.day) },
            { hat: 'bandana', date: new Date(y, 6, 4) },
            { hat: 'witch', date: new Date(y, 9, 31) },
            { hat: 'pilgrim', date: new Date(y, 10, thanksgivingDay(y)) },
            { hat: 'santa', date: new Date(y, 11, 25) }
        ];
    }

    function usHoliday(now) {
        var today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
        var best = null, bestDiff = HOLIDAY_WINDOW_DAYS + 1;
        for (var y = today.getFullYear() - 1; y <= today.getFullYear() + 1; y++) {
            holidaysFor(y).forEach(function (h) {
                var diff = Math.abs(Math.round((h.date - today) / 86400000));
                if (diff < bestDiff) {
                    best = h.hat;
                    bestDiff = diff;
                }
            });
        }
        return best;
    }

    var HAT_WIDTH = 0.36;
    var HEAD_X = 0.88;
    var BRIM_Y = 0.15;
    var llamaBox = null;

    function measureLlama() {
        var c = window.llamaChart;
        if (!c || !c.series || !c.series.length) return;
        var slices = c.series.getIndex(0).slices.values;
        if (!slices.length || slices.some(function (s) { return s.isHidden || s.visible === false; })) return;

        var f = fx.getBoundingClientRect();
        var left = Infinity, right = -Infinity, top = Infinity, bottom = -Infinity;
        slices.forEach(function (s) {
            var b = s.element.node.getBoundingClientRect();
            if (!b.width && !b.height) return;
            left = Math.min(left, b.left);
            right = Math.max(right, b.right);
            top = Math.min(top, b.top);
            bottom = Math.max(bottom, b.bottom);
        });
        if (!isFinite(left) || right - left < 20) return;

        llamaBox = { x: left - f.left, y: top - f.top, w: right - left, h: bottom - top };
        placeHat();
    }

    function placeHat() {
        var el = fx.querySelector('.llama-hat');
        if (!el || !llamaBox) return;
        el.style.width = (llamaBox.w * HAT_WIDTH).toFixed(1) + 'px';
        el.style.left = (llamaBox.x + llamaBox.w * HEAD_X).toFixed(1) + 'px';
        el.style.top = (llamaBox.y + llamaBox.h * BRIM_Y).toFixed(1) + 'px';
        el.style.visibility = 'visible';
    }

    var measureTimer = null;
    function scheduleMeasure(delay) {
        clearTimeout(measureTimer);
        measureTimer = setTimeout(measureLlama, delay);
    }

    document.addEventListener('llama:ready', function () { scheduleMeasure(2800); });
    if (window.llamaChart) scheduleMeasure(2800);
    window.addEventListener('resize', function () { scheduleMeasure(500); });

    function addHat(name) {
        var old = fx.querySelector('.llama-hat');
        if (old) old.remove();
        var hat = HATS[name];
        if (!hat) return;
        var el = document.createElement('div');
        el.className = 'llama-hat';
        el.setAttribute('data-hat', name);
        el.style.visibility = 'hidden';
        el.innerHTML = hat.svg;
        fx.appendChild(el);
        placeHat();
    }

    var select = document.getElementById('llama-hat-select');
    var queryHat = new URLSearchParams(window.location.search).get('hat');
    var savedHat = null;
    try { savedHat = localStorage.getItem('llamaHat'); } catch (err) {}

    function applyChoice(choice) {
        if (choice === 'none') {
            addHat(null);
        } else if (HATS[choice]) {
            addHat(choice);
        } else {
            addHat(usHoliday(new Date()));
        }
    }

    var initial = HATS[queryHat] ? queryHat : (savedHat || 'auto');
    if (select) {
        select.value = initial;
        if (select.value !== initial) {
            initial = 'auto';
            select.value = 'auto';
        }
        select.addEventListener('change', function () {
            applyChoice(select.value);
            try { localStorage.setItem('llamaHat', select.value); } catch (err) {}
        });
    }
    applyChoice(initial);

    if (!reduceMotion) {
        stage.addEventListener('mousemove', function (e) {
            var r = stage.getBoundingClientRect();
            var dx = ((e.clientX - r.left) / r.width) * 2 - 1;
            var lean = 'translateX(' + (dx * 6).toFixed(1) + 'px) rotate(' + (dx * 2).toFixed(2) + 'deg)';
            chart.style.transform = lean;
            fx.style.transform = lean;
        });

        stage.addEventListener('mouseleave', function () {
            chart.style.transform = '';
            fx.style.transform = '';
        });

        stage.addEventListener('click', function (e) {
            var r = stage.getBoundingClientRect();
            var x = e.clientX - r.left, y = e.clientY - r.top;
            for (var i = 0; i < 12; i++) {
                var p = document.createElement('span');
                var angle = Math.random() * Math.PI * 2;
                var distance = 30 + Math.random() * 40;
                p.className = 'llama-pixel';
                p.style.left = x + 'px';
                p.style.top = y + 'px';
                p.style.background = pick(colors);
                p.style.setProperty('--dx', (Math.cos(angle) * distance).toFixed(0) + 'px');
                p.style.setProperty('--dy', (Math.sin(angle) * distance).toFixed(0) + 'px');
                fx.appendChild(p);
                removeLater(p, 900);
            }
        });

        setInterval(function () {
            if (document.hidden || fx.children.length > 40 || Math.random() > 0.35) return;
            var r = stage.getBoundingClientRect();
            var p = document.createElement('span');
            p.className = 'llama-dust';
            p.style.left = (r.width * (0.36 + Math.random() * 0.28)) + 'px';
            p.style.top = (r.height * (0.6 + Math.random() * 0.08)) + 'px';
            p.style.background = pick(colors);
            fx.appendChild(p);
            removeLater(p, 2600);
        }, 400);
    }

    function walkerSvg() {
        var ns = 'http://www.w3.org/2000/svg';
        var probe = document.createElementNS(ns, 'svg');
        var path = document.createElementNS(ns, 'path');
        path.setAttribute('d', LLAMA_PATH);
        probe.setAttribute('style', 'position:absolute;visibility:hidden');
        probe.appendChild(path);
        document.body.appendChild(probe);
        var b = path.getBBox();
        probe.remove();
        return '<svg viewBox="' + b.x + ' ' + b.y + ' ' + b.width + ' ' + b.height + '"><path d="' + LLAMA_PATH + '" fill="#9F8CD7"/></svg>';
    }

    function walkOnce() {
        var walker = document.createElement('div');
        walker.className = 'llama-walker';
        walker.innerHTML = walkerSvg();
        document.body.appendChild(walker);
        setTimeout(function () { walker.classList.add('looking'); }, 4200);
        setTimeout(function () { walker.classList.remove('looking'); }, 10000);
        removeLater(walker, 14500);
    }

    if (!reduceMotion && 'IntersectionObserver' in window) {
        var observer = new IntersectionObserver(function (entries) {
            if (!entries[0].isIntersecting) return;
            observer.disconnect();
            walkOnce();
        }, { threshold: 0.5 });
        observer.observe(stage);
    }
})();
