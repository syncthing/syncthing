module.exports = function(grunt) {

  grunt.initConfig({
    pkg: grunt.file.readJSON("package.json"),
    sass: {
      dist: {
        options: {
          style: "expanded"
        },
        files: {
          "./assets/css/styles.css": "./assets/css/settings.scss"
        }
      }
    },

    uglify: {
      my_target: {
        options: {
          sourceMap: true,
          sourceMapIncludeSources: true,
          compress: {
            drop_console: false
          }
        },
        files: {
          "./assets/scripts/scripts.min.js": [ "./assets/scripts/scripts.js" ]
        }
      }
    },

    watch: {
      css: {
        files: ["./assets/css/sass/*/*.scss", "./assets/css/*.scss"],
        tasks: "sass"
      },
      js: {
        files: "./assets/scripts/*.js",
        tasks: "uglify"
      }
    }
  });

  // Load sass
  grunt.loadNpmTasks("grunt-sass");

  // Load uglify
  grunt.loadNpmTasks("grunt-contrib-uglify");

  // Load watch
  grunt.loadNpmTasks("grunt-contrib-watch");

  // Default tasks
  grunt.registerTask("default", ["sass", "uglify"]);

};
