// Разработка: быстрые пересборки, HMR с React Fast Refresh, proxy /api → gateway бэкенда.
// SPA и API на одном origin (localhost:3000) — cookie сессии plenka_sid работает без CORS.
const { merge } = require('webpack-merge');
const ReactRefreshWebpackPlugin = require('@pmmmwh/react-refresh-webpack-plugin');
const common = require('./webpack.common.cjs');

module.exports = merge(common, {
  mode: 'development',
  devtool: 'eval-cheap-module-source-map',
  output: { filename: '[name].js' },
  module: {
    rules: [{ test: /\.css$/, use: ['style-loader', 'css-loader'] }],
  },
  // Горячая замена React-компонентов с сохранением состояния (react-refresh).
  // Работает вместе с трансформером react-refresh-typescript в ts-loader (webpack.common.cjs).
  plugins: [new ReactRefreshWebpackPlugin({ overlay: false })],
  devServer: {
    port: 3000,
    hot: true, // HMR; для React-компонентов его дополняет Fast Refresh (плагин ниже)
    historyApiFallback: true,
    proxy: [
      {
        context: ['/api'],
        target: process.env.VITE_GATEWAY_URL || 'http://localhost:8080',
        changeOrigin: true,
      },
    ],
  },
});
