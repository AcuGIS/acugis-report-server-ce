import assert from 'node:assert/strict';
import fs from 'fs';
import path from 'path';
import test from 'node:test';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const mapStylePath = path.join(root, 'assets', 'js', 'map-style.js');
const spatialPath = path.join(root, 'assets', 'js', 'spatial-selection.js');
const viewerPath = path.join(root, 'go-report-server', 'internal', 'server', 'viewer.html');

class MockSource {
  constructor(opts) {
    this.opts = opts || {};
    this.attributions = this.opts.attributions || '';
    this.disposed = false;
    this.cleared = false;
  }
  clear() {
    this.cleared = true;
  }
  dispose() {
    this.disposed = true;
  }
  getAttributions() {
    return this.attributions;
  }
  setAttributions(value) {
    this.attributions = value;
  }
}

class MockLayer {
  constructor(opts) {
    opts = opts || {};
    this.props = Object.assign({}, opts.properties || {});
    this.zIndex = opts.zIndex || 0;
    this.source = opts.source || null;
    this.children = [];
    this.disposed = false;
  }
  get(key) {
    return this.props[key];
  }
  set(key, value) {
    this.props[key] = value;
  }
  setZIndex(z) {
    this.zIndex = z;
  }
  getSource() {
    return this.source;
  }
  getLayers() {
    const children = this.children;
    return {
      getArray() {
        return children;
      },
      forEach(fn) {
        children.forEach(fn);
      },
    };
  }
  dispose() {
    this.disposed = true;
  }
}

class MockPolygon {
  constructor(rings) {
    this.rings = rings;
  }
  getExtent() {
    const pts = this.rings[0];
    const xs = pts.map((p) => p[0]);
    const ys = pts.map((p) => p[1]);
    return [Math.min(...xs), Math.min(...ys), Math.max(...xs), Math.max(...ys)];
  }
  clone() {
    return new MockPolygon(this.rings.map((ring) => ring.map((p) => p.slice())));
  }
  transform() {
    return this;
  }
}

function installOpenLayers() {
  const applied = [];
  const ol = {
    layer: {
      Tile: MockLayer,
      Group: MockLayer,
      Vector: MockLayer,
      Heatmap: MockLayer,
    },
    source: {
      XYZ: MockSource,
      Vector: class {
        constructor() {
          this.features = [];
        }
        clear() {
          this.features = [];
        }
        addFeature(feature) {
          this.features.push(feature);
        }
        removeFeature(feature) {
          this.features = this.features.filter((item) => item !== feature);
        }
        getFeatures() {
          return this.features.slice();
        }
        forEachFeatureIntersectingExtent() {}
      },
    },
    style: {
      Style: class {},
      Fill: class {},
      Stroke: class {},
    },
    geom: {
      Polygon: class extends MockPolygon {
        static fromExtent(extent) {
          return new MockPolygon([[
            [extent[0], extent[1]],
            [extent[2], extent[1]],
            [extent[2], extent[3]],
            [extent[0], extent[3]],
            [extent[0], extent[1]],
          ]]);
        }
      },
    },
    extent: {
      getWidth(extent) {
        return extent[2] - extent[0];
      },
      getHeight(extent) {
        return extent[3] - extent[1];
      },
    },
    proj: {
      transformExtent(extent) {
        return extent.slice();
      },
    },
    format: {
      GeoJSON: class {
        writeGeometryObject() {
          return { type: 'Polygon', coordinates: [] };
        }
      },
    },
    interaction: {
      Pointer: class {
        constructor(handlers) {
          this.handlers = handlers;
        }
        setActive() {}
      },
      DragPan: class {},
    },
    Feature: class {
      constructor(opts) {
        this.geometry = opts && opts.geometry;
      }
      getGeometry() {
        return this.geometry;
      }
      setGeometry(geometry) {
        this.geometry = geometry;
      }
    },
  };
  globalThis.ol = ol;
  globalThis.olms = {
    apply(group, url) {
      applied.push({ group, url });
      const child = new MockLayer({
        zIndex: 80,
        source: new MockSource({}),
      });
      group.children.push(child);
      return Promise.resolve();
    },
  };
  globalThis.window = globalThis;
  if (typeof globalThis.addEventListener !== 'function') {
    globalThis.addEventListener = function () {};
    globalThis.removeEventListener = function () {};
  }
  return { ol, applied };
}

installOpenLayers();
vm.runInThisContext(fs.readFileSync(mapStylePath, 'utf8'), { filename: mapStylePath });
vm.runInThisContext(fs.readFileSync(spatialPath, 'utf8'), { filename: spatialPath });

const style = globalThis.JrsMapStyle;
const { applied } = { applied: null };

function freshApplied() {
  globalThis.olms.apply = function (group, url) {
    const child = new MockLayer({ zIndex: 80, source: new MockSource({}) });
    group.children.push(child);
    freshApplied.urls.push(url);
    return Promise.resolve();
  };
  freshApplied.urls = [];
}

test('new map report uses the OpenFreeMap Positron default', () => {
  assert.equal(style.DEFAULT_MAP_REPORT_BASEMAP, 'ofm_positron');
  assert.equal(style.normalizeMapReportBasemap(undefined), 'ofm_positron');
  assert.equal(style.normalizeMapReportBasemap(''), 'ofm_positron');
});

test('legacy CARTO and OSM basemaps map to OpenFreeMap styles', () => {
  assert.equal(style.normalizeMapReportBasemap('carto_light'), 'ofm_positron');
  assert.equal(style.normalizeMapReportBasemap('Carto Light'), 'ofm_positron');
  assert.equal(style.normalizeMapReportBasemap('carto-light'), 'ofm_positron');
  assert.equal(style.normalizeMapReportBasemap('carto_dark'), 'ofm_dark');
  assert.equal(style.normalizeMapReportBasemap('Carto Dark'), 'ofm_dark');
  assert.equal(style.normalizeMapReportBasemap('carto-dark'), 'ofm_dark');
  assert.equal(style.normalizeMapReportBasemap('carto_voyager'), 'ofm_bright');
  assert.equal(style.normalizeMapReportBasemap('Carto Voyager'), 'ofm_bright');
  assert.equal(style.normalizeMapReportBasemap('carto-voyager'), 'ofm_bright');
  assert.equal(style.normalizeMapReportBasemap('osm'), 'ofm_positron');
  assert.equal(style.normalizeMapReportBasemap('OpenStreetMap'), 'ofm_positron');
  assert.equal(
    style.normalizeMapReportBasemap('https://tile.openstreetmap.org/{z}/{x}/{y}.png'),
    'ofm_positron'
  );
});

test('OpenFreeMap basemap is a style group, not an XYZ tile URL', async () => {
  freshApplied();
  const layer = style.createBasemapLayer({ basemap: 'ofm_liberty' });
  assert.equal(layer.get('role'), 'basemap');
  assert.equal(layer.get('basemap'), 'ofm_liberty');
  assert.equal(layer.get('style'), 'https://tiles.openfreemap.org/styles/liberty');
  assert.equal(layer.source, null);
  await layer.get('styleApply');
  assert.deepEqual(freshApplied.urls, ['https://tiles.openfreemap.org/styles/liberty']);
  assert.equal(layer.children.length, 1);
  assert.equal(layer.children[0].zIndex, 0);
  assert.match(String(layer.children[0].getSource().attributions), /OpenFreeMap/);
  assert.match(String(layer.children[0].getSource().attributions), /OpenStreetMap/);
});

test('switching basemap removes and stops the previous layer', async () => {
  freshApplied();
  const data = new MockLayer({ properties: { role: 'data' }, zIndex: 10 });
  const highlight = new MockLayer({ properties: { role: 'highlight' }, zIndex: 20 });
  const layers = [];
  const map = {
    getLayers() {
      return {
        getArray() {
          return layers;
        },
        remove(layer) {
          const index = layers.indexOf(layer);
          if (index >= 0) {
            layers.splice(index, 1);
          }
        },
        insertAt(index, layer) {
          layers.splice(index, 0, layer);
        },
      };
    },
  };
  const first = style.createBasemapLayer({ basemap: 'carto_light' });
  layers.push(first, data, highlight);
  await first.get('styleApply');
  const next = style.replaceBasemapLayer(map, { basemap: 'carto_dark' });
  await next.get('styleApply');
  const basemaps = layers.filter((layer) => layer.get('role') === 'basemap');
  assert.equal(basemaps.length, 1);
  assert.equal(basemaps[0], next);
  assert.equal(next.get('basemap'), 'ofm_dark');
  assert.equal(first.get('basemapStopped'), true);
  assert.equal(first.disposed, true);
  assert.equal(layers.includes(data), true);
  assert.equal(layers.includes(highlight), true);
  assert.equal(layers[0], next);
  assert.deepEqual(freshApplied.urls, [
    'https://tiles.openfreemap.org/styles/positron',
    'https://tiles.openfreemap.org/styles/dark',
  ]);
});

test('heatmap overlay stays above the basemap and feature redraw does not require a new basemap', () => {
  freshApplied();
  const basemap = style.createBasemapLayer({});
  const built = style.createDataLayers(
    { renderer: 'heatmap', field: 'value' },
    new globalThis.ol.source.Vector(),
    [{ get: (key) => (key === 'value' ? 3 : null) }]
  );
  assert.equal(built.layers.length, 1);
  assert.equal(built.layers[0].get('role'), 'data');
  assert.ok(built.layers[0].zIndex > basemap.zIndex);
  assert.equal(freshApplied.urls.length, 1);
});

test('rectangle, circle, and clear selection still report through the controller', () => {
  let pointer;
  const mapEl = {
    style: {},
    classList: { add() {}, remove() {} },
    addEventListener() {},
  };
  const map = {
    addLayer() {},
    removeLayer() {},
    addInteraction(interaction) {
      pointer = interaction;
    },
    removeInteraction() {},
    getInteractions() {
      return { forEach() {} };
    },
    getTargetElement() {
      return mapEl;
    },
  };
  const buttons = ['rectangle', 'circle', 'clear'].map((tool) => ({
    disabled: false,
    tool,
    classList: { toggle() {}, remove() {} },
    getAttribute(name) {
      return name === 'data-tool' ? tool : null;
    },
    closest(selector) {
      return selector === '[data-tool]' ? this : null;
    },
  }));
  let onClick;
  const toolbar = {
    hidden: false,
    addEventListener(type, fn) {
      if (type === 'click') {
        onClick = fn;
      }
    },
    querySelectorAll() {
      return buttons;
    },
  };
  const selections = [];
  let cleared = 0;
  const controller = globalThis.JrsSpatialSelection.createController({
    map,
    ol: globalThis.ol,
    toolbarEl: toolbar,
    vectorSource: new globalThis.ol.source.Vector(),
    onSelection(selection) {
      selections.push(selection);
    },
    onClear() {
      cleared += 1;
    },
  });
  onClick({ target: buttons[0] });
  pointer.handlers.handleDownEvent({ coordinate: [0, 0], originalEvent: { button: 0 } });
  pointer.handlers.handleUpEvent({ coordinate: [100, 80], originalEvent: { button: 0 } });
  assert.equal(selections.at(-1).type, 'rectangle');

  onClick({ target: buttons[1] });
  pointer.handlers.handleDownEvent({ coordinate: [10, 10], originalEvent: { button: 0 } });
  pointer.handlers.handleDragEvent({ coordinate: [40, 10] });
  pointer.handlers.handleUpEvent({ coordinate: [40, 10], originalEvent: { button: 0 } });
  assert.equal(selections.at(-1).type, 'circle');
  assert.equal(selections.at(-1).geometry.type, 'Polygon');

  onClick({ target: buttons[2] });
  assert.equal(cleared, 1);
  assert.equal(controller.getSelectionGeometry(), null);
  assert.equal(controller.isToolActive(), false);
});

test('viewer keeps parameter, selection, and report refresh behavior', () => {
  const html = fs.readFileSync(viewerPath, 'utf8');
  assert.match(html, /id="applyBtn"/);
  assert.match(html, /id="resetBtn"/);
  assert.match(html, /applyFilterContext\(\{ clearSpatial: true, fit: true \}\)/);
  assert.match(html, /filterContext\.p_geometry = body\.p_geometry/);
  assert.match(html, /filterContext\.p_srid = body\.p_srid/);
  assert.match(html, /data-tool="rectangle"/);
  assert.match(html, /data-tool="circle"/);
  assert.match(html, /data-tool="clear"/);
  assert.match(html, /if \(role === 'data' \|\| role === 'labels'\)/);
  assert.match(html, /previousBasemapKey !== nextBasemapKey/);
  assert.match(html, /exportReport\('pdf'\)/);
  assert.doesNotMatch(html, /cartocdn\.com/);
  assert.doesNotMatch(html, /tile\.openstreetmap\.org/);
  assert.match(html, /MapReportCSP/);
  const cspGo = fs.readFileSync(
    path.join(root, 'go-report-server', 'internal', 'spatial', 'basemap.go'),
    'utf8'
  );
  const cspFn = cspGo.slice(cspGo.indexOf('func MapReportViewerCSP'));
  assert.match(cspFn, /https:\/\/tiles\.openfreemap\.org/);
  assert.doesNotMatch(cspFn, /\*/);
  assert.doesNotMatch(cspFn, /cartocdn\.com/);
  assert.doesNotMatch(cspFn, /tile\.openstreetmap\.org/);
});

test('map style source does not keep CARTO or OSM raster URLs', () => {
  const source = fs.readFileSync(mapStylePath, 'utf8');
  assert.doesNotMatch(source, /url:\s*'https:\/\/[^']*cartocdn/);
  assert.doesNotMatch(source, /url:\s*'https:\/\/[^']*tile\.openstreetmap\.org/);
  assert.doesNotMatch(source, /source\.OSM/);
  assert.match(source, /https:\/\/tiles\.openfreemap\.org\/styles\//);
  assert.match(source, /OPENFREEMAP_STYLE_BASE \+ 'positron'/);
  assert.match(source, /OPENFREEMAP_STYLE_BASE \+ 'liberty'/);
  assert.match(source, /OPENFREEMAP_STYLE_BASE \+ 'bright'/);
  assert.match(source, /OPENFREEMAP_STYLE_BASE \+ 'dark'/);
});

void applied;
