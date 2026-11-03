/**
 * Map Report OpenLayers style / layer builder.
 * All visual appearance is derived from the report mapDefinition JSON.
 * Do not hardcode fill/stroke/label styling outside of mapDefinition defaults.
 */
(function (global) {
  'use strict';

  const COLOR_RAMPS = {
    Blues: ['#f7fbff', '#c6dbef', '#6baed6', '#2171b5', '#08306b'],
    Greens: ['#f7fcf5', '#c7e9c0', '#74c476', '#238b45', '#00441b'],
    Reds: ['#fff5f0', '#fcbba1', '#fb6a4a', '#cb181d', '#67000d'],
    Viridis: ['#440154', '#3b528b', '#21918c', '#5ec962', '#fde725'],
    Spectral: ['#9e0142', '#f46d43', '#fee08b', '#66c2a5', '#5e4fa2'],
  };

  function parseColor(hex, opacity) {
    const h = String(hex || '#3388ff').replace('#', '');
    const full = h.length === 3 ? h.split('').map((c) => c + c).join('') : h;
    const n = parseInt(full, 16);
    if (!Number.isFinite(n)) {
      return `rgba(51,136,255,${opacity == null ? 1 : Number(opacity)})`;
    }
    const r = (n >> 16) & 255;
    const g = (n >> 8) & 255;
    const b = n & 255;
    const a = opacity == null ? 1 : Number(opacity);
    return `rgba(${r},${g},${b},${a})`;
  }

  function rampColors(name, classes) {
    const base = COLOR_RAMPS[name] || COLOR_RAMPS.Blues;
    const n = Math.max(2, Math.min(12, Number(classes) || 5));
    if (n === base.length) {
      return base.slice();
    }
    const out = [];
    for (let i = 0; i < n; i++) {
      const t = n === 1 ? 0 : i / (n - 1);
      const idx = t * (base.length - 1);
      const lo = Math.floor(idx);
      const hi = Math.min(base.length - 1, lo + 1);
      out.push(t === 0 || lo === hi ? base[lo] : base[hi]);
    }
    return out;
  }

  function collectNumericValues(features, field) {
    const values = [];
    features.forEach((f) => {
      const n = Number(f.get(field));
      if (Number.isFinite(n)) {
        values.push(n);
      }
    });
    return values.sort((a, b) => a - b);
  }

  function equalIntervalBreaks(values, classes) {
    if (!values.length) {
      return [];
    }
    const min = values[0];
    const max = values[values.length - 1];
    const n = Math.max(2, Math.min(12, Number(classes) || 5));
    if (min === max) {
      return [max];
    }
    const step = (max - min) / n;
    const breaks = [];
    for (let i = 1; i <= n; i++) {
      breaks.push(min + step * i);
    }
    return breaks;
  }

  function quantileBreaks(values, classes) {
    if (!values.length) {
      return [];
    }
    const n = Math.max(2, Math.min(12, Number(classes) || 5));
    const breaks = [];
    for (let i = 1; i <= n; i++) {
      const q = i / n;
      const pos = (values.length - 1) * q;
      const lo = Math.floor(pos);
      const hi = Math.ceil(pos);
      const v = lo === hi ? values[lo] : values[lo] + (values[hi] - values[lo]) * (pos - lo);
      breaks.push(v);
    }
    return breaks;
  }

  function stdDevBreaks(values, classes) {
    if (!values.length) {
      return [];
    }
    const n = Math.max(2, Math.min(12, Number(classes) || 5));
    const mean = values.reduce((s, v) => s + v, 0) / values.length;
    const variance = values.reduce((s, v) => s + (v - mean) * (v - mean), 0) / values.length;
    const sd = Math.sqrt(variance) || 1;
    const half = Math.floor(n / 2);
    const breaks = [];
    for (let i = -half + 1; i <= half; i++) {
      breaks.push(mean + i * sd);
    }
    while (breaks.length < n) {
      breaks.push(mean + (breaks.length - half) * sd);
    }
    return breaks.slice(0, n).sort((a, b) => a - b);
  }

  /** Simple 1D Jenks natural breaks. */
  function naturalBreaks(values, classes) {
    if (!values.length) {
      return [];
    }
    const data = values.slice();
    const n = Math.max(2, Math.min(12, Number(classes) || 5));
    if (data.length <= n) {
      return data.slice();
    }

    const matrices = [];
    const lowerClassLimits = [];
    for (let i = 0; i <= data.length; i++) {
      const row1 = [];
      const row2 = [];
      for (let j = 0; j <= n; j++) {
        row1.push(0);
        row2.push(0);
      }
      matrices.push(row1);
      lowerClassLimits.push(row2);
    }

    for (let i = 1; i <= n; i++) {
      matrices[1][i] = 1;
      lowerClassLimits[1][i] = 1;
      for (let j = 2; j <= data.length; j++) {
        matrices[j][i] = Infinity;
      }
    }

    let variance = 0;
    for (let l = 2; l <= data.length; l++) {
      let sum = 0;
      let sumSquares = 0;
      let w = 0;
      for (let m = 1; m <= l; m++) {
        const i3 = l - m + 1;
        const val = data[i3 - 1];
        sumSquares += val * val;
        sum += val;
        w += 1;
        variance = sumSquares - (sum * sum) / w;
        const i4 = i3 - 1;
        if (i4 !== 0) {
          for (let j = 2; j <= n; j++) {
            if (matrices[l][j] >= variance + matrices[i4][j - 1]) {
              lowerClassLimits[l][j] = i3;
              matrices[l][j] = variance + matrices[i4][j - 1];
            }
          }
        }
      }
      lowerClassLimits[l][1] = 1;
      matrices[l][1] = variance;
    }

    const kclass = [];
    let k = data.length;
    for (let j = 0; j < n; j++) {
      kclass[j] = 0;
    }
    kclass[n - 1] = data[data.length - 1];
    let countNum = n;
    while (countNum >= 2) {
      const id = lowerClassLimits[k][countNum] - 2;
      kclass[countNum - 2] = data[Math.max(0, id)];
      k = lowerClassLimits[k][countNum] - 1;
      countNum -= 1;
    }
    return kclass.filter((v, i, arr) => i === 0 || v !== arr[i - 1]);
  }

  function computeBreaks(features, field, classification, classes) {
    const values = collectNumericValues(features, field);
    switch (classification) {
      case 'equal_interval':
        return equalIntervalBreaks(values, classes);
      case 'standard_deviation':
        return stdDevBreaks(values, classes);
      case 'natural_breaks':
        return naturalBreaks(values, classes);
      case 'quantile':
      default:
        return quantileBreaks(values, classes);
    }
  }

  function classIndex(value, breaks) {
    const num = Number(value);
    if (!Number.isFinite(num) || !breaks.length) {
      return breaks.length ? breaks.length - 1 : 0;
    }
    for (let i = 0; i < breaks.length; i++) {
      if (num <= breaks[i]) {
        return i;
      }
    }
    return breaks.length - 1;
  }

  function geometryKind(feature) {
    const g = feature.getGeometry && feature.getGeometry();
    const type = g ? String(g.getType() || '') : '';
    if (type.indexOf('Point') >= 0) return 'point';
    if (type.indexOf('Line') >= 0) return 'line';
    return 'polygon';
  }

  const POINT_SHAPES = ['circle', 'square', 'triangle', 'diamond', 'star', 'cross'];

  function normalizePointShape(raw) {
    const key = String(raw || 'circle').toLowerCase().replace(/[\s-]+/g, '_');
    return POINT_SHAPES.indexOf(key) >= 0 ? key : 'circle';
  }

  function graduatedRadii(classCount) {
    const preset = [4, 6, 8, 11, 14];
    const n = Math.max(3, Math.min(7, Number(classCount) || 5));
    if (n === preset.length) {
      return preset.slice();
    }
    const out = [];
    for (let i = 0; i < n; i++) {
      const t = n === 1 ? 0 : i / (n - 1);
      out.push(Math.round(preset[0] + t * (preset[preset.length - 1] - preset[0])));
    }
    return out;
  }

  function classRadius(classIndex, classCount) {
    const radii = graduatedRadii(classCount);
    const idx = Math.max(0, Math.min(radii.length - 1, classIndex));
    return radii[idx];
  }

  function createPointImage(OL, shape, radius, fill, stroke) {
    const r = Math.max(2, Number(radius) || 6);
    const normalized = normalizePointShape(shape);
    if (normalized === 'circle') {
      return new OL.style.Circle({ radius: r, fill: fill, stroke: stroke });
    }
    if (normalized === 'square') {
      return new OL.style.RegularShape({
        points: 4,
        radius: r,
        angle: Math.PI / 4,
        fill: fill,
        stroke: stroke,
      });
    }
    if (normalized === 'triangle') {
      return new OL.style.RegularShape({
        points: 3,
        radius: r,
        fill: fill,
        stroke: stroke,
      });
    }
    if (normalized === 'diamond') {
      return new OL.style.RegularShape({
        points: 4,
        radius: r,
        fill: fill,
        stroke: stroke,
      });
    }
    if (normalized === 'star') {
      return new OL.style.RegularShape({
        points: 5,
        radius: r,
        radius2: r * 0.45,
        fill: fill,
        stroke: stroke,
      });
    }
    if (normalized === 'cross') {
      return new OL.style.RegularShape({
        points: 4,
        radius: r,
        radius2: 0,
        stroke: stroke,
        fill: fill,
      });
    }
    return new OL.style.Circle({ radius: r, fill: fill, stroke: stroke });
  }

  function singleStyle(OL, symbol, kind, overrides) {
    const fill = (overrides && overrides.fillColor) || symbol.fillColor || '#26a69a';
    const outline = (overrides && overrides.outlineColor) || symbol.outlineColor || '#00796b';
    const width = Number((overrides && overrides.outlineWidth) != null ? overrides.outlineWidth : symbol.outlineWidth) || 1.5;
    const opacity = (overrides && overrides.opacity) != null ? Number(overrides.opacity) : Number(symbol.opacity);
    const a = Number.isFinite(opacity) ? opacity : 0.35;
    const radius = (overrides && overrides.radius) != null ? Number(overrides.radius) : 6;
    const shape = normalizePointShape((overrides && overrides.shape) || symbol.shape || 'circle');

    if (kind === 'point') {
      return new OL.style.Style({
        image: createPointImage(
          OL,
          shape,
          radius,
          new OL.style.Fill({ color: parseColor(fill, Math.min(1, a + 0.4)) }),
          new OL.style.Stroke({ color: parseColor(outline, 1), width: width })
        ),
      });
    }
    if (kind === 'line') {
      return new OL.style.Style({
        stroke: new OL.style.Stroke({
          color: parseColor(outline, Math.max(a, 0.7)),
          width: Math.max(width, 2),
        }),
      });
    }
    return new OL.style.Style({
      fill: new OL.style.Fill({ color: parseColor(fill, a) }),
      stroke: new OL.style.Stroke({ color: parseColor(outline, 1), width: width }),
    });
  }

  function labelStyle(OL, feature, labels) {
    if (!labels || !labels.enabled || !labels.field) {
      return null;
    }
    const text = feature.get(labels.field);
    if (text == null || text === '') {
      return null;
    }
    const opts = {
      text: String(text),
      font: (labels.fontSize || 12) + 'px Roboto, sans-serif',
      fill: new OL.style.Fill({ color: labels.fillColor || '#212121' }),
      overflow: true,
    };
    if (labels.halo) {
      opts.stroke = new OL.style.Stroke({ color: '#ffffff', width: 3 });
    }
    return new OL.style.Style({ text: new OL.style.Text(opts) });
  }

  /**
   * Build an OpenLayers style function (and optional runtime class breaks) from mapDefinition.
   */
  function buildStyleFromDefinition(mapDefinition, features) {
    const OL = global.ol;
    const def = mapDefinition || {};
    const renderer = String(def.renderer || 'single');
    const symbol = def.singleSymbol || {};
    const field = def.field || null;
    const labels = def.labels || { enabled: false };

    let breaks = [];
    let colors = [];
    let valueMin = 0;
    let valueMax = 1;
    let graduatedRadiiList = [];
    if (renderer === 'graduated' && field) {
      breaks = computeBreaks(features || [], field, def.classification || 'quantile', def.classes || 5);
      colors = rampColors(def.colorRamp || 'Blues', Math.max(breaks.length, def.classes || 5));
      graduatedRadiiList = graduatedRadii(Math.max(breaks.length, def.classes || 5));
      const nums = collectNumericValues(features || [], field);
      if (nums.length) {
        valueMin = nums[0];
        valueMax = nums[nums.length - 1];
      }
    } else if (renderer === 'choropleth' && field) {
      breaks = computeBreaks(features || [], field, def.classification || 'quantile', def.classes || 5);
      colors = rampColors(def.colorRamp || 'Blues', Math.max(breaks.length, def.classes || 5));
      const nums = collectNumericValues(features || [], field);
      if (nums.length) {
        valueMin = nums[0];
        valueMax = nums[nums.length - 1];
      }
    }

    if (renderer === 'proportional' && field && features && features.length) {
      const nums = collectNumericValues(features, field);
      if (nums.length) {
        valueMin = nums[0];
        valueMax = nums[nums.length - 1];
        if (valueMin === valueMax) {
          valueMax = valueMin + 1;
        }
      }
    }

    const categories = Array.isArray(def.categories) ? def.categories.slice() : [];
    if (renderer === 'categorized' && field && categories.length === 0 && features && features.length) {
      const seen = {};
      const values = [];
      features.forEach((f) => {
        const v = f.get(field);
        const key = v == null ? '' : String(v);
        if (!seen[key]) {
          seen[key] = true;
          values.push(key);
        }
      });
      values.sort();
      const colors = rampColors(def.colorRamp || 'Blues', Math.max(values.length, 2));
      values.forEach((value, i) => {
        categories.push({
          value: value,
          label: value === '' ? '(empty)' : value,
          fillColor: colors[Math.min(i, colors.length - 1)] || '#9e9e9e',
          outlineColor: symbol.outlineColor || '#424242',
        });
      });
    }

    return {
      breaks: breaks,
      colors: colors,
      valueMin: valueMin,
      valueMax: valueMax,
      categories: categories,
      graduatedRadii: graduatedRadiiList,
      styleFunction: function (feature) {
        const kind = geometryKind(feature);
        const styles = [];
        let base;

        if (renderer === 'categorized' && field) {
          const key = feature.get(field);
          const keyStr = key == null ? '' : String(key);
          let match = categories.find((c) => String(c.value) === keyStr);
          if (!match) {
            match = categories.find((c) => c.value === '__other__') || {
              fillColor: '#9e9e9e',
              outlineColor: symbol.outlineColor || '#424242',
            };
          }
          base = singleStyle(OL, symbol, kind, {
            fillColor: match.fillColor || match.color,
            outlineColor: match.outlineColor || symbol.outlineColor,
            opacity: symbol.opacity,
            shape: symbol.shape,
          });
        } else if (renderer === 'graduated' && field && breaks.length) {
          const idx = classIndex(feature.get(field), breaks);
          const color = colors[Math.min(idx, colors.length - 1)] || colors[0] || '#2171b5';
          const overrides = {
            fillColor: color,
            outlineColor: symbol.outlineColor || '#263238',
            opacity: symbol.opacity,
            shape: symbol.shape,
          };
          if (kind === 'point') {
            overrides.radius = classRadius(idx, breaks.length);
          }
          base = singleStyle(OL, symbol, kind, overrides);
        } else if (renderer === 'choropleth' && field && breaks.length) {
          const idx = classIndex(feature.get(field), breaks);
          const color = colors[Math.min(idx, colors.length - 1)] || colors[0] || '#2171b5';
          if (kind === 'point') {
            base = singleStyle(OL, symbol, kind);
          } else {
            base = singleStyle(OL, symbol, kind, {
              fillColor: color,
              outlineColor: symbol.outlineColor || '#263238',
              opacity: symbol.opacity,
            });
          }
        } else if (renderer === 'proportional' && field) {
          const num = Number(feature.get(field));
          const t = Number.isFinite(num) ? (num - valueMin) / (valueMax - valueMin) : 0.5;
          const radius = 4 + Math.max(0, Math.min(1, t)) * 20;
          base = singleStyle(OL, symbol, 'point', {
            fillColor: symbol.fillColor,
            outlineColor: symbol.outlineColor,
            opacity: symbol.opacity,
            radius: radius,
            outlineWidth: symbol.outlineWidth,
          });
        } else {
          base = singleStyle(OL, symbol, kind);
        }

        styles.push(base);
        const lbl = labelStyle(OL, feature, labels);
        if (lbl) {
          styles.push(lbl);
        }
        return styles;
      },
    };
  }

  /** Single default for new reports, empty values, and unresolvable legacy basemaps. */
  const DEFAULT_MAP_REPORT_BASEMAP = 'ofm_positron';

  /** Required when the client is not MapLibre. TileJSON repeats this string. */
  const OPENFREEMAP_ATTRIBUTION = 'OpenFreeMap © OpenMapTiles Data from OpenStreetMap';

  const OPENFREEMAP_STYLE_BASE = 'https://tiles.openfreemap.org/styles/';

  const BASEMAP_SOURCES = {
    ofm_positron: {
      type: 'openfreemap',
      style: OPENFREEMAP_STYLE_BASE + 'positron',
    },
    ofm_liberty: {
      type: 'openfreemap',
      style: OPENFREEMAP_STYLE_BASE + 'liberty',
    },
    ofm_bright: {
      type: 'openfreemap',
      style: OPENFREEMAP_STYLE_BASE + 'bright',
    },
    ofm_dark: {
      type: 'openfreemap',
      style: OPENFREEMAP_STYLE_BASE + 'dark',
    },
    esri_streets: {
      type: 'xyz',
      url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Street_Map/MapServer/tile/{z}/{y}/{x}',
      attributions: 'Tiles © Esri',
      maxZoom: 19,
    },
    esri_imagery: {
      type: 'xyz',
      url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
      attributions: 'Tiles © Esri',
      maxZoom: 19,
    },
  };

  const BASEMAP_ALIASES = {
    osm: DEFAULT_MAP_REPORT_BASEMAP,
    openstreetmap: DEFAULT_MAP_REPORT_BASEMAP,
    open_street_map: DEFAULT_MAP_REPORT_BASEMAP,
    carto_light: 'ofm_positron',
    carto: 'ofm_positron',
    light: 'ofm_positron',
    light_all: 'ofm_positron',
    positron: 'ofm_positron',
    ofm_positron: 'ofm_positron',
    openfreemap_positron: 'ofm_positron',
    carto_dark: 'ofm_dark',
    dark: 'ofm_dark',
    dark_all: 'ofm_dark',
    dark_matter: 'ofm_dark',
    carto_dark_matter: 'ofm_dark',
    ofm_dark: 'ofm_dark',
    openfreemap_dark: 'ofm_dark',
    carto_voyager: 'ofm_bright',
    voyager: 'ofm_bright',
    bright: 'ofm_bright',
    ofm_bright: 'ofm_bright',
    openfreemap_bright: 'ofm_bright',
    liberty: 'ofm_liberty',
    ofm_liberty: 'ofm_liberty',
    openfreemap_liberty: 'ofm_liberty',
    esri_streets: 'esri_streets',
    esri_topo: 'esri_streets',
    streets: 'esri_streets',
    terrain: 'esri_streets',
    topo: 'esri_streets',
    esri_imagery: 'esri_imagery',
    esri_satellite: 'esri_imagery',
    satellite: 'esri_imagery',
    imagery: 'esri_imagery',
  };

  function foldBasemapKey(saved) {
    return String(saved || '')
      .trim()
      .toLowerCase()
      .replace(/[-_]+/g, ' ')
      .replace(/\s+/g, ' ')
      .trim()
      .replace(/ /g, '_');
  }

  /**
   * Resolve a saved Map Report basemap to a supported id.
   * Stored reports are not rewritten here.
   */
  function normalizeMapReportBasemap(saved) {
    const raw = String(saved || '').trim();
    if (!raw) {
      return DEFAULT_MAP_REPORT_BASEMAP;
    }
    const lower = raw.toLowerCase();
    if (lower.indexOf('tiles.openfreemap.org/styles/') !== -1) {
      if (lower.indexOf('/styles/dark') !== -1 || lower.indexOf('/styles/fiord') !== -1) {
        return 'ofm_dark';
      }
      if (lower.indexOf('/styles/bright') !== -1) {
        return 'ofm_bright';
      }
      if (lower.indexOf('/styles/liberty') !== -1) {
        return 'ofm_liberty';
      }
      if (lower.indexOf('/styles/positron') !== -1) {
        return 'ofm_positron';
      }
      return DEFAULT_MAP_REPORT_BASEMAP;
    }
    if (lower.indexOf('tile.openstreetmap.org') !== -1) {
      return DEFAULT_MAP_REPORT_BASEMAP;
    }
    if (lower.indexOf('cartocdn.com') !== -1) {
      if (lower.indexOf('dark_all') !== -1 || lower.indexOf('dark_matter') !== -1 || lower.indexOf('dark-matter') !== -1) {
        return 'ofm_dark';
      }
      if (lower.indexOf('voyager') !== -1) {
        return 'ofm_bright';
      }
      return 'ofm_positron';
    }
    return BASEMAP_ALIASES[foldBasemapKey(raw)] || DEFAULT_MAP_REPORT_BASEMAP;
  }

  function clampBasemapZIndex(layer) {
    if (!layer) {
      return;
    }
    if (typeof layer.setZIndex === 'function') {
      layer.setZIndex(0);
    }
    const collection = layer.getLayers && layer.getLayers();
    const children = collection && collection.getArray ? collection.getArray() : [];
    children.forEach(clampBasemapZIndex);
  }

  const stoppedBasemaps = new WeakSet();

  function basemapIsStopped(layer) {
    return !layer || stoppedBasemaps.has(layer) || layer.get('basemapStopped');
  }

  function stopBasemapLayer(layer) {
    if (!layer) {
      return;
    }
    const already = basemapIsStopped(layer);
    stoppedBasemaps.add(layer);
    layer.set('basemapStopped', true);
    const collection = layer.getLayers && layer.getLayers();
    const children = collection && collection.getArray ? collection.getArray().slice() : [];
    children.forEach(stopBasemapLayer);
    if (already) {
      return;
    }
    const source = layer.getSource && layer.getSource();
    if (source) {
      if (typeof source.clear === 'function') {
        try {
          source.clear();
        } catch (err) {
          /* source may already be disposed */
        }
      }
      if (typeof source.dispose === 'function') {
        try {
          source.dispose();
        } catch (err) {
          /* source may already be disposed */
        }
      }
    }
    if (typeof layer.dispose === 'function') {
      try {
        layer.dispose();
      } catch (err) {
        /* layer may already be disposed */
      }
    }
  }

  function attributionText(source) {
    if (!source || typeof source.getAttributions !== 'function') {
      return '';
    }
    const attr = source.getAttributions();
    if (attr == null) {
      return '';
    }
    if (typeof attr === 'string') {
      return attr;
    }
    if (Array.isArray(attr)) {
      return attr.join(' ');
    }
    if (typeof attr === 'function') {
      try {
        const value = attr();
        return Array.isArray(value) ? value.join(' ') : String(value || '');
      } catch (err) {
        return '';
      }
    }
    return '';
  }

  function stampOpenFreeMapAttribution(layer) {
    if (!layer) {
      return;
    }
    const source = layer.getSource && layer.getSource();
    if (source && typeof source.setAttributions === 'function') {
      const existing = attributionText(source);
      if (existing.indexOf('OpenFreeMap') === -1 && existing.indexOf('OpenStreetMap') === -1) {
        source.setAttributions(OPENFREEMAP_ATTRIBUTION);
      }
    }
    const collection = layer.getLayers && layer.getLayers();
    const children = collection && collection.getArray ? collection.getArray() : [];
    children.forEach(stampOpenFreeMapAttribution);
  }

  function applyOpenFreeMapStyle(group, styleUrl) {
    const olmsApi = global.olms;
    if (!olmsApi || typeof olmsApi.apply !== 'function') {
      console.warn('OpenFreeMap style engine (ol-mapbox-style) is not loaded.');
      return Promise.resolve();
    }
    let pending;
    try {
      pending = olmsApi.apply(group, styleUrl);
    } catch (err) {
      console.warn('OpenFreeMap style failed', err);
      return Promise.resolve();
    }
    const promise = Promise.resolve(pending).then(function () {
      if (basemapIsStopped(group)) {
        stopBasemapLayer(group);
        return;
      }
      clampBasemapZIndex(group);
      stampOpenFreeMapAttribution(group);
    }).catch(function (err) {
      if (!group.get('basemapStopped')) {
        console.warn('OpenFreeMap style failed', err);
      }
    });
    group.set('styleApply', promise);
    return promise;
  }

  function createBasemapLayer(mapDefinition) {
    const OL = global.ol;
    const key = normalizeMapReportBasemap(mapDefinition && mapDefinition.basemap);
    const cfg = BASEMAP_SOURCES[key] || BASEMAP_SOURCES[DEFAULT_MAP_REPORT_BASEMAP];

    if (cfg.type === 'xyz') {
      return new OL.layer.Tile({
        source: new OL.source.XYZ({
          url: cfg.url,
          attributions: cfg.attributions,
          maxZoom: cfg.maxZoom || 19,
          crossOrigin: 'anonymous',
        }),
        properties: { role: 'basemap', basemap: key },
        zIndex: 0,
      });
    }

    // OpenFreeMap styles are MapLibre documents. ol-mapbox-style turns one
    // into an OpenLayers layer group; do not pass the style URL to XYZ/OSM.
    const group = new OL.layer.Group({
      properties: { role: 'basemap', basemap: key, style: cfg.style },
      zIndex: 0,
    });
    applyOpenFreeMapStyle(group, cfg.style);
    return group;
  }

  /**
   * Remove every current basemap layer, then add one replacement at the bottom.
   * Data, label, highlight, and selection layers are left in place.
   */
  function replaceBasemapLayer(map, mapDefinition) {
    if (!map || typeof map.getLayers !== 'function') {
      return null;
    }
    const collection = map.getLayers();
    const existing = collection.getArray().filter(function (layer) {
      return layer.get('role') === 'basemap';
    });
    existing.forEach(function (layer) {
      collection.remove(layer);
      stopBasemapLayer(layer);
    });
    const next = createBasemapLayer(mapDefinition);
    collection.insertAt(0, next);
    return next;
  }

  function formatLegendNumber(n) {
    if (!Number.isFinite(n)) {
      return '—';
    }
    const abs = Math.abs(n);
    if (abs >= 1000000) {
      return (n / 1000000).toFixed(1).replace(/\.0$/, '') + 'M';
    }
    if (abs >= 1000) {
      return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
    }
    if (Number.isInteger(n) || Math.abs(n - Math.round(n)) < 1e-6) {
      return String(Math.round(n));
    }
    return n.toFixed(abs >= 10 ? 1 : 2);
  }

  /**
   * Build legend HTML for thematic renderers.
   * Returns empty string for single-symbol (no legend).
   */
  function legendSymbolRow(color, label, shape, radius) {
    const r = Math.max(4, Number(radius) || 8);
    const normalized = normalizePointShape(shape || 'circle');
    let symbolHtml;
    if (normalized === 'circle') {
      symbolHtml =
        '<span class="jrs-legend-circle" style="width:' +
        r * 2 +
        'px;height:' +
        r * 2 +
        'px;background:' +
        escapeHtml(color) +
        ';"></span>';
    } else {
      symbolHtml =
        '<span class="jrs-legend-shape jrs-legend-shape-' +
        escapeHtml(normalized) +
        '" style="width:' +
        r * 2 +
        'px;height:' +
        r * 2 +
        'px;background:' +
        escapeHtml(color) +
        ';"></span>';
    }
    return (
      '<div class="jrs-legend-row jrs-legend-symbol">' +
      symbolHtml +
      '<span class="jrs-legend-label">' +
      escapeHtml(String(label)) +
      '</span></div>'
    );
  }

  function buildLegendHtml(mapDefinition, styleMeta) {
    const def = mapDefinition || {};
    const renderer = String(def.renderer || 'single');
    const thematic = ['categorized', 'graduated', 'choropleth', 'heatmap', 'proportional'];
    if (!thematic.includes(renderer)) {
      return '';
    }

    const meta = styleMeta || {};
    const field = def.field || '';
    const title = field ? escapeHtml(field) : escapeHtml(renderer);
    const rows = [];
    const pointShape = (def.singleSymbol && def.singleSymbol.shape) || 'circle';
    const usePointLegend = meta.dominantGeometryKind === 'point' || meta.dominantGeometryKind === 'mixed';

    if (renderer === 'categorized') {
      const categories = (meta.categories && meta.categories.length)
        ? meta.categories
        : (Array.isArray(def.categories) ? def.categories : []);
      categories.forEach((cat) => {
        const color = cat.fillColor || cat.color || '#9e9e9e';
        const label = cat.label || cat.value || '';
        if (usePointLegend) {
          rows.push(legendSymbolRow(color, label, pointShape, 6));
        } else {
          rows.push(legendSwatchRow(color, label));
        }
      });
      if (!rows.length) {
        rows.push('<div class="jrs-legend-empty">No categories defined</div>');
      }
    } else if (renderer === 'graduated' || renderer === 'choropleth') {
      const breaks = meta.breaks || [];
      const colors = meta.colors || rampColors(def.colorRamp || 'Blues', def.classes || 5);
      const radii = meta.graduatedRadii || graduatedRadii(breaks.length || def.classes || 5);
      let prev = Number.isFinite(meta.valueMin) ? meta.valueMin : null;
      for (let i = 0; i < breaks.length; i++) {
        const hi = breaks[i];
        const color = colors[Math.min(i, colors.length - 1)] || '#2171b5';
        let label;
        if (prev == null) {
          label = '≤ ' + formatLegendNumber(hi);
        } else {
          label = formatLegendNumber(prev) + ' – ' + formatLegendNumber(hi);
        }
        if (renderer === 'graduated' && usePointLegend) {
          rows.push(legendSymbolRow(color, label, pointShape, radii[i] || classRadius(i, breaks.length)));
        } else {
          rows.push(legendSwatchRow(color, label));
        }
        prev = hi;
      }
      if (!rows.length) {
        rows.push('<div class="jrs-legend-empty">No classes</div>');
      }
    } else if (renderer === 'heatmap') {
      const lo = formatLegendNumber(meta.valueMin);
      const hi = formatLegendNumber(meta.valueMax);
      rows.push(
        '<div class="jrs-legend-ramp">' +
          '<div class="jrs-legend-ramp-bar"></div>' +
          '<div class="jrs-legend-ramp-labels"><span>' +
          escapeHtml(lo) +
          '</span><span>' +
          escapeHtml(hi) +
          '</span></div></div>'
      );
    } else if (renderer === 'proportional') {
      const fill = (def.singleSymbol && def.singleSymbol.fillColor) || '#26a69a';
      const outline = (def.singleSymbol && def.singleSymbol.outlineColor) || '#00796b';
      const min = meta.valueMin;
      const max = meta.valueMax;
      const mid = Number.isFinite(min) && Number.isFinite(max) ? (min + max) / 2 : null;
      const samples = [
        { r: 6, label: formatLegendNumber(min) },
        { r: 12, label: mid != null ? formatLegendNumber(mid) : '—' },
        { r: 18, label: formatLegendNumber(max) },
      ];
      samples.forEach((s) => {
        rows.push(
          '<div class="jrs-legend-row jrs-legend-prop">' +
            '<span class="jrs-legend-circle" style="width:' +
            s.r * 2 +
            'px;height:' +
            s.r * 2 +
            'px;background:' +
            escapeHtml(fill) +
            ';border-color:' +
            escapeHtml(outline) +
            ';"></span>' +
            '<span class="jrs-legend-label">' +
            escapeHtml(s.label) +
            '</span></div>'
        );
      });
    }

    return (
      '<div class="jrs-map-legend" role="region" aria-label="Map legend">' +
      '<div class="jrs-legend-title">' +
      title +
      '</div>' +
      '<div class="jrs-legend-body">' +
      rows.join('') +
      '</div></div>'
    );
  }

  function legendSwatchRow(color, label) {
    return (
      '<div class="jrs-legend-row">' +
      '<span class="jrs-legend-swatch" style="background:' +
      escapeHtml(color) +
      ';"></span>' +
      '<span class="jrs-legend-label">' +
      escapeHtml(String(label)) +
      '</span></div>'
    );
  }

  /**
   * Create the data layer(s) for the map from mapDefinition + vector source.
   * Heatmap uses ol/layer/Heatmap; other renderers use Vector with a style function.
   */
  function createDataLayers(mapDefinition, vectorSource, features) {
    const OL = global.ol;
    const def = mapDefinition || {};
    const renderer = String(def.renderer || 'single');
    const layers = [];

    if (renderer === 'heatmap') {
      const weightField = def.field || null;
      let wMin = 0;
      let wMax = 1;
      if (weightField && features && features.length) {
        const nums = collectNumericValues(features, weightField);
        if (nums.length) {
          wMin = nums[0];
          wMax = nums[nums.length - 1];
          if (wMin === wMax) {
            wMax = wMin + 1;
          }
        }
      }
      layers.push(
        new OL.layer.Heatmap({
          source: vectorSource,
          blur: 15,
          radius: 10,
          weight: function (feature) {
            if (!weightField) {
              return 1;
            }
            const n = Number(feature.get(weightField));
            if (!Number.isFinite(n)) {
              return 0.5;
            }
            return Math.max(0, Math.min(1, (n - wMin) / (wMax - wMin)));
          },
        })
      );
      layers[layers.length - 1].set('role', 'data');
      layers[layers.length - 1].setZIndex(10);
      // Labels on a transparent vector overlay when enabled.
      if (def.labels && def.labels.enabled) {
        const labelLayer = new OL.layer.Vector({
          source: vectorSource,
          style: function (feature) {
            const lbl = labelStyle(OL, feature, def.labels);
            return lbl ? [lbl] : [];
          },
        });
        labelLayer.set('role', 'labels');
        labelLayer.setZIndex(15);
        layers.push(labelLayer);
      }
      return { layers: layers, styleMeta: { breaks: [], colors: [], valueMin: wMin, valueMax: wMax } };
    }

    const built = buildStyleFromDefinition(def, features);
    let dominantGeometryKind = 'unknown';
    if (features && features.length) {
      const counts = { point: 0, line: 0, polygon: 0 };
      features.forEach(function (f) {
        const k = geometryKind(f);
        if (counts[k] != null) {
          counts[k]++;
        }
      });
      const sorted = Object.keys(counts).sort(function (a, b) {
        return counts[b] - counts[a];
      });
      const top = sorted[0];
      const nonZero = sorted.filter(function (k) {
        return counts[k] > 0;
      });
      dominantGeometryKind = nonZero.length > 1 ? 'mixed' : top || 'unknown';
    }
    const dataLayer = new OL.layer.Vector({
      source: vectorSource,
      style: built.styleFunction,
    });
    dataLayer.set('role', 'data');
    dataLayer.setZIndex(10);
    // Ensure polygons remain hittable for popups even with low fill opacity.
    dataLayer.set('renderBuffer', 128);
    layers.push(dataLayer);
    return {
      layers: layers,
      styleMeta: {
        breaks: built.breaks,
        colors: built.colors,
        valueMin: built.valueMin,
        valueMax: built.valueMax,
        categories: built.categories || [],
        graduatedRadii: built.graduatedRadii || [],
        dominantGeometryKind: dominantGeometryKind,
      },
    };
  }

  function featureProperty(feature, name) {
    if (!feature || name == null || name === '') {
      return undefined;
    }
    const key = String(name);
    let val = feature.get(key);
    if (val !== undefined && val !== null) {
      return val;
    }
    // Qualified UI labels may be stored as table.column while GeoJSON keys are bare column names.
    if (key.indexOf('.') >= 0) {
      const shortName = key.split('.').pop();
      if (shortName) {
        val = feature.get(shortName);
        if (val !== undefined && val !== null) {
          return val;
        }
      }
    }
    // Case-insensitive fallback for drivers that fold identifiers.
    const keys = feature.getKeys ? feature.getKeys() : [];
    const lower = key.toLowerCase();
    for (let i = 0; i < keys.length; i++) {
      if (String(keys[i]).toLowerCase() === lower) {
        return feature.get(keys[i]);
      }
      if (key.indexOf('.') >= 0) {
        const shortName = key.split('.').pop();
        if (shortName && String(keys[i]).toLowerCase() === String(shortName).toLowerCase()) {
          return feature.get(keys[i]);
        }
      }
    }
    return undefined;
  }

  function buildPopupHtml(feature, mapDefinition) {
    const popups = (mapDefinition && mapDefinition.popups) || {};
    if (!popups.enabled) {
      return '';
    }
    const fields = Array.isArray(popups.fields) ? popups.fields : [];
    if (!fields.length) {
      return '';
    }
    const rows = fields
      .map((name) => {
        const val = featureProperty(feature, name);
        const display = val == null || val === '' ? '—' : String(val);
        return (
          '<tr><th style="text-align:left;padding:2px 8px 2px 0;color:#607d8b;font-weight:500;">' +
          escapeHtml(name) +
          '</th><td style="padding:2px 0;">' +
          escapeHtml(display) +
          '</td></tr>'
        );
      })
      .join('');
    return (
      '<div class="jrs-map-popup" style="font:13px Roboto,sans-serif;max-width:260px;">' +
      '<table style="border-collapse:collapse;">' +
      rows +
      '</table></div>'
    );
  }

  function buildTooltipText(feature, mapDefinition) {
    const tooltips = (mapDefinition && mapDefinition.tooltips) || {};
    if (!tooltips.enabled) {
      return '';
    }
    const fields = Array.isArray(tooltips.fields) ? tooltips.fields : [];
    if (!fields.length) {
      return '';
    }
    return fields
      .map((name) => {
        const val = featureProperty(feature, name);
        const display = val == null || val === '' ? '—' : String(val);
        return name + ': ' + display;
      })
      .join('\n');
  }

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function highlightStyle(OL) {
    return new OL.style.Style({
      fill: new OL.style.Fill({ color: 'rgba(255, 193, 7, 0.35)' }),
      stroke: new OL.style.Stroke({ color: '#ff6f00', width: 2.5 }),
      image: new OL.style.Circle({
        radius: 7,
        fill: new OL.style.Fill({ color: 'rgba(255, 193, 7, 0.8)' }),
        stroke: new OL.style.Stroke({ color: '#ff6f00', width: 2 }),
      }),
      zIndex: 10,
    });
  }

  global.JrsMapStyle = {
    parseColor: parseColor,
    rampColors: rampColors,
    computeBreaks: computeBreaks,
    buildStyleFromDefinition: buildStyleFromDefinition,
    DEFAULT_MAP_REPORT_BASEMAP: DEFAULT_MAP_REPORT_BASEMAP,
    OPENFREEMAP_ATTRIBUTION: OPENFREEMAP_ATTRIBUTION,
    normalizeMapReportBasemap: normalizeMapReportBasemap,
    createBasemapLayer: createBasemapLayer,
    replaceBasemapLayer: replaceBasemapLayer,
    createDataLayers: createDataLayers,
    buildPopupHtml: buildPopupHtml,
    buildTooltipText: buildTooltipText,
    buildLegendHtml: buildLegendHtml,
    highlightStyle: highlightStyle,
  };
})(typeof window !== 'undefined' ? window : globalThis);
