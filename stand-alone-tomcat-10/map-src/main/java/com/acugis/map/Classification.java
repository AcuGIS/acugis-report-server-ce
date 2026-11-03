package com.acugis.map;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/** Classification algorithms matching Server map-style.js (A+B). */
final class Classification {

    private Classification() {}

    static List<Double> computeBreaks(List<Double> sortedValues, Symbology.Classification method, int classes) {
        if (sortedValues == null || sortedValues.isEmpty()) {
            return Collections.emptyList();
        }
        int n = Math.max(2, Math.min(12, classes));
        switch (method) {
            case EQUAL_INTERVAL:
                return equalInterval(sortedValues, n);
            case STANDARD_DEVIATION:
                return stdDev(sortedValues, n);
            case NATURAL_BREAKS:
                return naturalBreaks(sortedValues, n);
            case QUANTILE:
            default:
                return quantile(sortedValues, n);
        }
    }

    static int classIndex(double value, List<Double> breaks) {
        if (breaks == null || breaks.isEmpty()) {
            return 0;
        }
        for (int i = 0; i < breaks.size(); i++) {
            if (value <= breaks.get(i)) {
                return i;
            }
        }
        return breaks.size() - 1;
    }

    private static List<Double> equalInterval(List<Double> values, int n) {
        double min = values.get(0);
        double max = values.get(values.size() - 1);
        if (min == max) {
            return Collections.singletonList(max);
        }
        double step = (max - min) / n;
        List<Double> breaks = new ArrayList<Double>(n);
        for (int i = 1; i <= n; i++) {
            breaks.add(min + step * i);
        }
        return breaks;
    }

    private static List<Double> quantile(List<Double> values, int n) {
        List<Double> breaks = new ArrayList<Double>(n);
        for (int i = 1; i <= n; i++) {
            double q = (double) i / (double) n;
            double pos = (values.size() - 1) * q;
            int lo = (int) Math.floor(pos);
            int hi = (int) Math.ceil(pos);
            double v = lo == hi ? values.get(lo)
                    : values.get(lo) + (values.get(hi) - values.get(lo)) * (pos - lo);
            breaks.add(v);
        }
        return breaks;
    }

    private static List<Double> stdDev(List<Double> values, int n) {
        double sum = 0;
        for (double v : values) {
            sum += v;
        }
        double mean = sum / values.size();
        double var = 0;
        for (double v : values) {
            var += (v - mean) * (v - mean);
        }
        var /= values.size();
        double sd = Math.sqrt(var);
        if (sd == 0) {
            sd = 1;
        }
        int half = n / 2;
        List<Double> breaks = new ArrayList<Double>();
        for (int i = -half + 1; i <= half; i++) {
            breaks.add(mean + i * sd);
        }
        while (breaks.size() < n) {
            breaks.add(mean + (breaks.size() - half) * sd);
        }
        if (breaks.size() > n) {
            breaks = new ArrayList<Double>(breaks.subList(0, n));
        }
        Collections.sort(breaks);
        return breaks;
    }

    /** Simple 1D Jenks natural breaks (Server map-style.js port). */
    private static List<Double> naturalBreaks(List<Double> values, int n) {
        List<Double> data = new ArrayList<Double>(values);
        if (data.size() <= n) {
            return new ArrayList<Double>(data);
        }
        int len = data.size();
        double[][] matrices = new double[len + 1][n + 1];
        int[][] lower = new int[len + 1][n + 1];

        for (int i = 1; i <= n; i++) {
            matrices[1][i] = 1;
            lower[1][i] = 1;
            for (int j = 2; j <= len; j++) {
                matrices[j][i] = Double.POSITIVE_INFINITY;
            }
        }

        for (int l = 2; l <= len; l++) {
            double sum = 0;
            double sumSquares = 0;
            double w = 0;
            double variance = 0;
            for (int m = 1; m <= l; m++) {
                int i3 = l - m + 1;
                double val = data.get(i3 - 1);
                sumSquares += val * val;
                sum += val;
                w += 1;
                variance = sumSquares - (sum * sum) / w;
                int i4 = i3 - 1;
                if (i4 != 0) {
                    for (int j = 2; j <= n; j++) {
                        if (matrices[l][j] >= variance + matrices[i4][j - 1]) {
                            lower[l][j] = i3;
                            matrices[l][j] = variance + matrices[i4][j - 1];
                        }
                    }
                }
            }
            lower[l][1] = 1;
            matrices[l][1] = variance;
        }

        double[] kclass = new double[n];
        kclass[n - 1] = data.get(len - 1);
        int k = len;
        int countNum = n;
        while (countNum >= 2) {
            int id = lower[k][countNum] - 2;
            kclass[countNum - 2] = data.get(Math.max(0, id));
            k = lower[k][countNum] - 1;
            countNum -= 1;
        }
        List<Double> out = new ArrayList<Double>();
        for (int i = 0; i < n; i++) {
            if (i == 0 || kclass[i] != kclass[i - 1]) {
                out.add(kclass[i]);
            }
        }
        return out;
    }
}
