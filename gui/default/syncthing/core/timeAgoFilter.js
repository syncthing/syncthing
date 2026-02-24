angular.module('syncthing.core')
.filter('timeAgo', function () {
    return function (input) {
        if (!input) {
            return '';
        }
        
        var momentObj = moment(input);
        
        if (!momentObj.isValid()) {
            return '';
        }
        
        return momentObj.fromNow();
    };
}); 