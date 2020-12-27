// Karma configuration
// Generated on Thu Nov 12 2020 02:43:34 GMT+0000 (Coordinated Universal Time)

module.exports = function(config) {
  config.set({
    // base path that will be used to resolve all patterns (eg. files, exclude)
    basePath: '',

    // available frameworks: https://npmjs.org/browse/keyword/karma-adapter
    frameworks: ['jasmine'],

    // list of files / patterns to load in the browser including external
    // dependencies, app, and tests.
    // Loaded in order, so we ensure each part of the syncthing app is loaded
    // before files that depend on them.
    files: [
        'gui/default/vendor/jquery/jquery-2.2.2.js',
        'gui/default/vendor/fancytree/jquery.fancytree-all-deps.js',
        'gui/default/vendor/angular/angular.js',
        './node_modules/angular-mocks/angular-mocks.js',
        'gui/default/syncthing/**/module.js',
        'gui/default/syncthing/folder/!(*.test).js',
        'gui/default/syncthing/**/*.test.js',
    ],

    // list of files / patterns to exclude
    exclude: [],

    // available reporters: https://npmjs.org/browse/keyword/karma-reporter
    reporters: ['mocha'],

    // web server port
    port: 9876,

    // level of logging
    // possible values: config.LOG_DISABLE || config.LOG_ERROR || config.LOG_WARN || config.LOG_INFO || config.LOG_DEBUG
    logLevel: config.LOG_INFO,

    // enable / disable watching file and executing tests whenever any file changes
    autoWatch: true,

    // start these browsers
    // available browser launchers: https://npmjs.org/browse/keyword/karma-launcher
    browsers: [],

    // Continuous Integration mode
    // if true, Karma captures browsers, runs the tests and exits
    singleRun: false,

    // Concurrency level
    // how many browser should be started simultaneous
    concurrency: Infinity
  })
}
