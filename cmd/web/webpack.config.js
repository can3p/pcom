const path = require('path');
const {CleanWebpackPlugin} = require('clean-webpack-plugin');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const OptimizeCSSAssetsPlugin = require('optimize-css-assets-webpack-plugin');
const TerserPlugin = require('terser-webpack-plugin');
const { WebpackManifestPlugin } = require('webpack-manifest-plugin');
const CopyPlugin = require('copy-webpack-plugin');

/**
 * Base webpack configuration
 *
 * @param env -> env parameters
 * @param argv -> CLI arguments, 'argv.mode' is the current webpack mode (development | production)
 * @returns object
 */
module.exports = (env, argv) => {
  let isProduction = (argv.mode === 'production');

  let config = {
    // entry files to compile (relative to the base dir)
    entry: [
      "./client/scss/index.scss",
      "./client/js/index.js",
    ],

    // enable development source maps
    // * will be overwritten by 'source-maps' in production mode
    devtool: "inline-source-map",

    // path to store compiled JS bundle
    output: {
      // bundle relative name
      filename: isProduction ? "js/app.[contenthash].js" : "js/app.js",

      chunkFilename: isProduction ? 'js/[name].[contenthash].chunk.bundle.js': 'js/[name].chunk.bundle.js',
      // base build directory
      path: path.resolve(__dirname, "dist"),
      // path to build relative asset links
      publicPath: "/static/"
    },

    // plugins configurations
    plugins: [
      // save compiled SCSS into separated CSS file
      new MiniCssExtractPlugin({
        filename: isProduction ? "css/style.[contenthash].css" : "css/style.css"
      }),

      // copy static assets directory
      new CopyPlugin({
        patterns: [{from: 'client/static', to: 'static'}]
      }),
      new WebpackManifestPlugin({ publicPath: "" }),
    ],

    // production mode optimization
    optimization: {
      minimizer: [
        // CSS optimizer
        new OptimizeCSSAssetsPlugin(),
        // JS optimizer by default
        new TerserPlugin(),
      ],
      // do not split vendor in a separate chunk, keep it close to
      // extracted async modules
      splitChunks: false,
    },

    // custom loaders configuration
    module: {
      rules: [
        // styles loader
        {
          test: /\.(sa|sc|c)ss$/,
          use: [
            MiniCssExtractPlugin.loader,
            "css-loader",
            "sass-loader"
          ],
        },

        {
            test: /\.woff2?$/,
            type: "asset/resource",
        }
      ]
    },
  };

  // PRODUCTION ONLY configuration
  if (isProduction) {
    config.plugins.push(
      // clean 'dist' directory
      new CleanWebpackPlugin(),
    );
  }

  return config;
};
