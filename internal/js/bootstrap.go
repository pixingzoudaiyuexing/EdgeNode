//go:build script

package js

const bootstrapSource = `
let gojs = {};
gojs.prepareNamespace = function (ns) {
	let pieces = ns.split(".");
	let parent = gojs;
	let index = 0;
	pieces.forEach(function (piece) {
		if (piece === "gojs" && index === 0) {
			return;
		}
		if (typeof parent[piece] === "undefined") {
			parent[piece] = {};
		}
		parent = parent[piece];
		index++;
	});
};
gojs.copyAttrs = function (destObj, srcObj) {
	if (typeof srcObj !== "object") {
		return;
	}
	for (let key in srcObj) {
		if (srcObj.hasOwnProperty(key)) {
			destObj[key] = srcObj[key];
		}
	}
};
let callbacks = [];
gojs.once = function (fn) {
	if (typeof fn === "function") {
		callbacks.push(fn);
	}
};
gojs.runOnce = function () {
	callbacks.forEach(function (fn) {
		try {
			fn();
		} catch (err) {
			let text = fn.toString();
			if (text.length > 100) {
				text = text.substr(0, 100) + "...";
			}
			throw new Error(text + ": " + err.toString());
		}
	});
};
`
