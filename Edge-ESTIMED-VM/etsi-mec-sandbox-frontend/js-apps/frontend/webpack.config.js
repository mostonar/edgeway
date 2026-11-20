const webpack = require('webpack');
const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const HtmlWebpackPlugin = require('html-webpack-plugin');

var miniCssPlugin = new MiniCssExtractPlugin({
  filename: 'bundle.css?[contenthash]'
});
var htmlPlugin = new HtmlWebpackPlugin({
  template: 'src/index.html',
  favicon: 'src/img/favicon.ico'
});

module.exports = env => {
  return  {
    mode: 'development',
    entry: [
      './src/js/app-controller.js'
    ],
    output: {
      filename: 'bundle.js?[contenthash]',
      path: path.resolve(__dirname, 'dist')
    },
    resolve: {
      fallback: {
        fs: false
      }
    },
    module: {
      rules: [
        {
          parser: {
            amd: false
          }
        },
        {
          test: /\.jsx?$/,
          exclude: /node_modules/,
          use: [
            {
              loader: 'babel-loader',
              options: {
                presets: [require.resolve('@babel/preset-react')]
              }
            }]
        },
        {
          test: /\.scss$/,
          use: [
            MiniCssExtractPlugin.loader,
            'css-loader',
            {
              loader: 'sass-loader',
              options: {
                sassOptions: {
                  includePaths: ['./node_modules/material-components-web/node_modules', './node_modules'],
                  quietDeps: true,
                  silenceDeprecations: ['import', 'global-builtin', 'color-functions', 'slash-div']
                }
              }
            }
          ]
        },
        {
          test: /\.css$/,
          use: [MiniCssExtractPlugin.loader, 'css-loader']
        },
        {
          test: /\.(png|svg|jpg|gif)$/,
          type: 'javascript/auto',
          use: [
            {
              loader: 'file-loader',
              options: {
                name: '[name].[ext]',
                outputPath: 'img',
                publicPath: 'img'
              }
            }]
        },
        {
          test: /\.(ttf|woff|woff2|eot)$/,
          type: 'javascript/auto',
          use: [
            {
              loader: 'file-loader',
              options: {
                name: '[name].[ext]',
                outputPath: 'icons',
                publicPath: 'icons'
              }
            }]
        },
        {
          test: /\.html$/,
          use: ['html-loader']
        }]
    },
    plugins: [
      miniCssPlugin,
      htmlPlugin,
      new webpack.DefinePlugin({
        __VERSION__: JSON.stringify('v0.0.0'),
        'process.env.LOG_LEVEL': JSON.stringify(process.env.LOG_LEVEL || '')
      })
    ],
    devServer: {
      proxy: {
        '/': {
          target: 'https://' + (env ? env.MEEP_HOST : ''),
          secure: false
        }
      }
    }
  };
};
