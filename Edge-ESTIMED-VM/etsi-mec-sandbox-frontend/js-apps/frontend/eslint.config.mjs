import globals from "globals";
import reactPlugin from "eslint-plugin-react";

export default [
    {
        files: ["src/**/*.js", "src/**/*.jsx"],
        plugins: {
            react: reactPlugin,
        },
        languageOptions: {
            ecmaVersion: 2018,
            sourceType: "module",
            parserOptions: {
                ecmaFeatures: { jsx: true }
            },
            globals: {
                ...globals.browser,
                ...globals.es6,
                Atomics: "readonly",
                SharedArrayBuffer: "readonly"
            }
        },
        settings: {
            react: {
                version: "16.8"
            }
        },
        rules: {
            "no-trailing-spaces": ["error", { "ignoreComments": true }],
            "indent": ["error", 2],
            "linebreak-style": ["error", "unix"],
            "curly": ["error", "all"],
            "comma-dangle": ["error", "never"],
            "eqeqeq": ["error", "always"],
            "quotes": ["error", "single"],
            "semi": ["error", "always"],
            "react/jsx-uses-vars": 2,
            "react/jsx-uses-react": "error"
        }
    }
];
