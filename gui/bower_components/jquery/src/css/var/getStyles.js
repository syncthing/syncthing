define(function() {
	return function( elem ) {
		return elem.ownerDocument.defaultView.getComputedStyle( elem, null );
	};
});
