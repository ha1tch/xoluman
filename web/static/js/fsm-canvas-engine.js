/*
 Finite State Machine Designer
 Original author: Evan Wallace — https://madebyevan.com/
 Source: https://github.com/evanw/fsm
 License: MIT (see below)

 This file is Seam's own extended fork of Wallace's original 2010
 engine (github.com/ha1tch's Seam AMS project), not the unmodified
 upstream — carried into xoluman verbatim from that fork, since the
 fork already does real, substantial work upstream's canvas doesn't:
 multiple layered interaction animations (link-drag pulse, snap-back
 on a failed connection, hover halos, group-selection halo, label
 repositioning, momentum-based zoom), rubber-band multi-select,
 per-link guard/action properties with their own editing dialog
 (linkProperties, a side-map keyed by link._id — the base engine has
 none of this), per-link custom colours, and JSON/SVG/LaTeX export.
 None of this was rebuilt for xoluman; reusing it directly is the
 point.

 One addition on top of the fork, made here for xoluman's own use:
 linkProperties gained a third field, `output`, alongside the fork's
 existing `guard`/`action` — xolu's own transition model has a
 distinct Output value separate from any action/side-effect text,
 which neither Wallace's original nor Seam's fork had a field for.
 Every place `guard`/`action` are read, written, saved to backup data,
 or restored from it was extended to carry `output` the same way,
 consistently.
*/


/*

 Copyright (c) 2010 Evan Wallace

 Permission is hereby granted, free of charge, to any person
 obtaining a copy of this software and associated documentation
 files (the "Software"), to deal in the Software without
 restriction, including without limitation the rights to use,
 copy, modify, merge, publish, distribute, sublicense, and/or sell
 copies of the Software, and to permit persons to whom the
 Software is furnished to do so, subject to the following
 conditions:

 The above copyright notice and this permission notice shall be
 included in all copies or substantial portions of the Software.

 THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
 OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
 NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
 HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
 WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
 FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
 OTHER DEALINGS IN THE SOFTWARE.
*/

function Link(a, b) {
	this.nodeA = a;
	this.nodeB = b;
	this.text = '';
	this.lineAngleAdjust = 0; // value to add to textAngle when link is straight line

	// make anchor point relative to the locations of nodeA and nodeB
	this.parallelPart = 0.5; // percentage from nodeA to nodeB
	this.perpendicularPart = 0; // pixels from line between nodeA and nodeB
}

Link.prototype.getAnchorPoint = function() {
	var dx = this.nodeB.x - this.nodeA.x;
	var dy = this.nodeB.y - this.nodeA.y;
	var scale = Math.sqrt(dx * dx + dy * dy);
	return {
		'x': this.nodeA.x + dx * this.parallelPart - dy * this.perpendicularPart / scale,
		'y': this.nodeA.y + dy * this.parallelPart + dx * this.perpendicularPart / scale
	};
};

Link.prototype.setAnchorPoint = function(x, y) {
	var dx = this.nodeB.x - this.nodeA.x;
	var dy = this.nodeB.y - this.nodeA.y;
	var scale = Math.sqrt(dx * dx + dy * dy);
	this.parallelPart = (dx * (x - this.nodeA.x) + dy * (y - this.nodeA.y)) / (scale * scale);
	this.perpendicularPart = (dx * (y - this.nodeA.y) - dy * (x - this.nodeA.x)) / scale;
	// snap to a straight line
	if(this.parallelPart > 0 && this.parallelPart < 1 && Math.abs(this.perpendicularPart) < snapToPadding) {
		this.lineAngleAdjust = (this.perpendicularPart < 0) * Math.PI;
		this.perpendicularPart = 0;
	}
};

Link.prototype.getEndPointsAndCircle = function() {
	if(this.perpendicularPart == 0) {
		var midX = (this.nodeA.x + this.nodeB.x) / 2;
		var midY = (this.nodeA.y + this.nodeB.y) / 2;
		var start = this.nodeA.closestPointOnCircle(midX, midY);
		var end = this.nodeB.closestPointOnCircle(midX, midY);
		return {
			'hasCircle': false,
			'startX': start.x,
			'startY': start.y,
			'endX': end.x,
			'endY': end.y,
		};
	}
	var anchor = this.getAnchorPoint();
	var circle = circleFromThreePoints(this.nodeA.x, this.nodeA.y, this.nodeB.x, this.nodeB.y, anchor.x, anchor.y);
	var isReversed = (this.perpendicularPart > 0);
	var reverseScale = isReversed ? 1 : -1;
	var _rA = this.nodeA._r != null ? this.nodeA._r : nodeRadius;
	var _rB = this.nodeB._r != null ? this.nodeB._r : nodeRadius;
	var startAngle = Math.atan2(this.nodeA.y - circle.y, this.nodeA.x - circle.x) - reverseScale * _rA / circle.radius;
	var endAngle = Math.atan2(this.nodeB.y - circle.y, this.nodeB.x - circle.x) + reverseScale * _rB / circle.radius;
	var startX = circle.x + circle.radius * Math.cos(startAngle);
	var startY = circle.y + circle.radius * Math.sin(startAngle);
	var endX = circle.x + circle.radius * Math.cos(endAngle);
	var endY = circle.y + circle.radius * Math.sin(endAngle);
	return {
		'hasCircle': true,
		'startX': startX,
		'startY': startY,
		'endX': endX,
		'endY': endY,
		'startAngle': startAngle,
		'endAngle': endAngle,
		'circleX': circle.x,
		'circleY': circle.y,
		'circleRadius': circle.radius,
		'reverseScale': reverseScale,
		'isReversed': isReversed,
	};
};

Link.prototype.draw = function(c) {
	var stuff = this.getEndPointsAndCircle();
	// draw arc
	c.beginPath();
	if(stuff.hasCircle) {
		c.arc(stuff.circleX, stuff.circleY, stuff.circleRadius, stuff.startAngle, stuff.endAngle, stuff.isReversed);
	} else {
		c.moveTo(stuff.startX, stuff.startY);
		c.lineTo(stuff.endX, stuff.endY);
	}
	c.stroke();
	// draw the head of the arrow
	if(stuff.hasCircle) {
		drawArrow(c, stuff.endX, stuff.endY, stuff.endAngle - stuff.reverseScale * (Math.PI / 2));
	} else {
		drawArrow(c, stuff.endX, stuff.endY, Math.atan2(stuff.endY - stuff.startY, stuff.endX - stuff.startX));
	}
	// draw the text
	if(stuff.hasCircle) {
		var startAngle = stuff.startAngle;
		var endAngle = stuff.endAngle;
		if(endAngle < startAngle) {
			endAngle += Math.PI * 2;
		}
		var textAngle = (startAngle + endAngle) / 2 + stuff.isReversed * Math.PI;
		var textX = stuff.circleX + stuff.circleRadius * Math.cos(textAngle);
		var textY = stuff.circleY + stuff.circleRadius * Math.sin(textAngle);
		_drawTextSinOverride = _smoothLabelSide(this, Math.sin(textAngle));
		_drawTextCosOverride = _smoothLabelCos(this, Math.cos(textAngle));
		drawText(c, this.text, textX, textY, textAngle, selectedObject == this);
		_drawTextSinOverride = null;
		_drawTextCosOverride = null;
	} else {
		var textX = (stuff.startX + stuff.endX) / 2;
		var textY = (stuff.startY + stuff.endY) / 2;
		var textAngle = Math.atan2(stuff.endX - stuff.startX, stuff.startY - stuff.endY);
		_drawTextSinOverride = _smoothLabelSide(this, Math.sin(textAngle));
		_drawTextCosOverride = _smoothLabelCos(this, Math.cos(textAngle));
		drawText(c, this.text, textX, textY, textAngle + this.lineAngleAdjust, selectedObject == this);
		_drawTextSinOverride = null;
		_drawTextCosOverride = null;
	}
};

Link.prototype.getMidpoint = function() {
	var stuff = this.getEndPointsAndCircle();
	if(stuff.hasCircle) {
		var startAngle = stuff.startAngle, endAngle = stuff.endAngle;
		if(endAngle < startAngle) endAngle += Math.PI * 2;
		var mid = (startAngle + endAngle) / 2 + stuff.isReversed * Math.PI;
		return { x: stuff.circleX + stuff.circleRadius * Math.cos(mid),
		         y: stuff.circleY + stuff.circleRadius * Math.sin(mid) };
	} else {
		return { x: (stuff.startX + stuff.endX) / 2, y: (stuff.startY + stuff.endY) / 2 };
	}
};

Link.prototype.drawArcOnly = function(c) {
	var stuff = this.getEndPointsAndCircle();
	c.beginPath();
	if(stuff.hasCircle) {
		c.arc(stuff.circleX, stuff.circleY, stuff.circleRadius, stuff.startAngle, stuff.endAngle, stuff.isReversed);
	} else {
		c.moveTo(stuff.startX, stuff.startY);
		c.lineTo(stuff.endX, stuff.endY);
	}
	c.stroke();
};

Link.prototype.containsPoint = function(x, y) {
	var stuff = this.getEndPointsAndCircle();
	if(stuff.hasCircle) {
		var dx = x - stuff.circleX;
		var dy = y - stuff.circleY;
		var distance = Math.sqrt(dx*dx + dy*dy) - stuff.circleRadius;
		if(Math.abs(distance) < hitTargetPadding) {
			var angle = Math.atan2(dy, dx);
			var startAngle = stuff.startAngle;
			var endAngle = stuff.endAngle;
			if(stuff.isReversed) {
				var temp = startAngle;
				startAngle = endAngle;
				endAngle = temp;
			}
			if(endAngle < startAngle) {
				endAngle += Math.PI * 2;
			}
			if(angle < startAngle) {
				angle += Math.PI * 2;
			} else if(angle > endAngle) {
				angle -= Math.PI * 2;
			}
			return (angle > startAngle && angle < endAngle);
		}
	} else {
		var dx = stuff.endX - stuff.startX;
		var dy = stuff.endY - stuff.startY;
		var length = Math.sqrt(dx*dx + dy*dy);
		var percent = (dx * (x - stuff.startX) + dy * (y - stuff.startY)) / (length * length);
		var distance = (dx * (y - stuff.startY) - dy * (x - stuff.startX)) / length;
		return (percent > 0 && percent < 1 && Math.abs(distance) < hitTargetPadding);
	}
	return false;
};

function Node(x, y) {
	this.x = x;
	this.y = y;
	this.mouseOffsetX = 0;
	this.mouseOffsetY = 0;
	this.isAcceptState = false;
	this.text = '';
	this.textOnly = false;
}

Node.prototype.setMouseStart = function(x, y) {
	this.mouseOffsetX = this.x - x;
	this.mouseOffsetY = this.y - y;
};

Node.prototype.setAnchorPoint = function(x, y) {
	this.x = x + this.mouseOffsetX;
	this.y = y + this.mouseOffsetY;
};

Node.prototype.draw = function(c) {
	if (this.textOnly) {
		drawText(c, this.text, this.x, this.y, null, selectedObject == this);
		return;
	}

	// draw the circle
	var r = this._r != null ? this._r : nodeRadius;
	if(this === _growingNode) {
		var _gp = Math.min(_growT / _growDur, 1);
		// cubic ease-out
		var _ge = 1 - Math.pow(1 - _gp, 3);
		r = Math.max(1, r * _ge);
	}
	c.beginPath();
	c.arc(this.x, this.y, r, 0, 2 * Math.PI, false);
	// Fill with canvas background so arcs passing behind nodes are hidden
	var savedStroke = c.strokeStyle;
	var savedLabel  = c.fillStyle;   // label colour from drawUsing
	c.fillStyle = seamNodeFill;
	c.fill();
	c.strokeStyle = savedStroke;
	c.stroke();
	c.fillStyle = savedLabel;  // restore for drawText

	// draw the text
	drawText(c, this.text, this.x, this.y, null, selectedObject == this);

	// draw a double circle for an accept state (Irish green accent)
	// Skip during main pass if this node's label is being edited (overlay handles it)
	var _isEditing = (selectedObject == this) && editingLabel && _suppressEditBox;
	if(this.isAcceptState && !_isEditing) {
		c.strokeStyle = seamAccentColor;
		c.beginPath();
		c.arc(this.x, this.y, r - 6, 0, 2 * Math.PI, false);
		c.stroke();
		c.strokeStyle = savedStroke;
	}
};

Node.prototype.closestPointOnCircle = function(x, y) {
	var r = this._r != null ? this._r : nodeRadius;
	var dx = x - this.x;
	var dy = y - this.y;
	var scale = Math.sqrt(dx * dx + dy * dy);
	return {
		'x': this.x + dx * r / scale,
		'y': this.y + dy * r / scale,
	};
};

Node.prototype.containsPoint = function(x, y) {
	var r = this._r != null ? this._r : nodeRadius;
	return (x - this.x)*(x - this.x) + (y - this.y)*(y - this.y) < r*r;
};

function SelfLink(node, mouse) {
	this.node = node;
	this.anchorAngle = 0;
	this.mouseOffsetAngle = 0;
	this.text = '';

	if(mouse) {
		this.setAnchorPoint(mouse.x, mouse.y);
	}
}

SelfLink.prototype.setMouseStart = function(x, y) {
	this.mouseOffsetAngle = this.anchorAngle - Math.atan2(y - this.node.y, x - this.node.x);
};

SelfLink.prototype.setAnchorPoint = function(x, y) {
	this.anchorAngle = Math.atan2(y - this.node.y, x - this.node.x) + this.mouseOffsetAngle;
	// snap to 90 degrees
	var snap = Math.round(this.anchorAngle / (Math.PI / 2)) * (Math.PI / 2);
	if(Math.abs(this.anchorAngle - snap) < 0.1) this.anchorAngle = snap;
	// keep in the range -pi to pi so our containsPoint() function always works
	if(this.anchorAngle < -Math.PI) this.anchorAngle += 2 * Math.PI;
	if(this.anchorAngle > Math.PI) this.anchorAngle -= 2 * Math.PI;
};

SelfLink.prototype.getEndPointsAndCircle = function() {
	var _nr = this.node._r != null ? this.node._r : nodeRadius;
	var circleX = this.node.x + 1.5 * _nr * Math.cos(this.anchorAngle);
	var circleY = this.node.y + 1.5 * _nr * Math.sin(this.anchorAngle);
	var circleRadius = 0.75 * _nr;
	var startAngle = this.anchorAngle - Math.PI * 0.8;
	var endAngle = this.anchorAngle + Math.PI * 0.8;
	var startX = circleX + circleRadius * Math.cos(startAngle);
	var startY = circleY + circleRadius * Math.sin(startAngle);
	var endX = circleX + circleRadius * Math.cos(endAngle);
	var endY = circleY + circleRadius * Math.sin(endAngle);
	return {
		'hasCircle': true,
		'startX': startX,
		'startY': startY,
		'endX': endX,
		'endY': endY,
		'startAngle': startAngle,
		'endAngle': endAngle,
		'circleX': circleX,
		'circleY': circleY,
		'circleRadius': circleRadius
	};
};

SelfLink.prototype.draw = function(c) {
	var stuff = this.getEndPointsAndCircle();
	// draw arc
	c.beginPath();
	c.arc(stuff.circleX, stuff.circleY, stuff.circleRadius, stuff.startAngle, stuff.endAngle, false);
	c.stroke();
	// draw the text on the loop farthest from the node
	var textX = stuff.circleX + stuff.circleRadius * Math.cos(this.anchorAngle);
	var textY = stuff.circleY + stuff.circleRadius * Math.sin(this.anchorAngle);
	drawText(c, this.text, textX, textY, this.anchorAngle, selectedObject == this);
	// draw the head of the arrow
	drawArrow(c, stuff.endX, stuff.endY, stuff.endAngle + Math.PI * 0.4);
};

SelfLink.prototype.getMidpoint = function() {
	var stuff = this.getEndPointsAndCircle();
	return { x: stuff.circleX + stuff.circleRadius * Math.cos(this.anchorAngle),
	         y: stuff.circleY + stuff.circleRadius * Math.sin(this.anchorAngle) };
};

SelfLink.prototype.drawArcOnly = function(c) {
	var stuff = this.getEndPointsAndCircle();
	c.beginPath();
	c.arc(stuff.circleX, stuff.circleY, stuff.circleRadius, stuff.startAngle, stuff.endAngle, false);
	c.stroke();
};

SelfLink.prototype.containsPoint = function(x, y) {
	var stuff = this.getEndPointsAndCircle();
	var dx = x - stuff.circleX;
	var dy = y - stuff.circleY;
	var distance = Math.sqrt(dx*dx + dy*dy) - stuff.circleRadius;
	return (Math.abs(distance) < hitTargetPadding);
};

function StartLink(node, start) {
	this.node = node;
	this.deltaX = 0;
	this.deltaY = 0;
	this.text = '';

	if(start) {
		this.setAnchorPoint(start.x, start.y);
	}
}

StartLink.prototype.setAnchorPoint = function(x, y) {
	this.deltaX = x - this.node.x;
	this.deltaY = y - this.node.y;

	if(Math.abs(this.deltaX) < snapToPadding) {
		this.deltaX = 0;
	}

	if(Math.abs(this.deltaY) < snapToPadding) {
		this.deltaY = 0;
	}
};

StartLink.prototype.getEndPoints = function() {
	var startX = this.node.x + this.deltaX;
	var startY = this.node.y + this.deltaY;
	var end = this.node.closestPointOnCircle(startX, startY);
	return {
		'startX': startX,
		'startY': startY,
		'endX': end.x,
		'endY': end.y,
	};
};

StartLink.prototype.draw = function(c) {
	var stuff = this.getEndPoints();

	// draw the line
	c.beginPath();
	c.moveTo(stuff.startX, stuff.startY);
	c.lineTo(stuff.endX, stuff.endY);
	c.stroke();

	// draw the text at the end without the arrow
	var textAngle = Math.atan2(stuff.startY - stuff.endY, stuff.startX - stuff.endX);
	drawText(c, this.text, stuff.startX, stuff.startY, textAngle, selectedObject == this);

	// draw the head of the arrow
	drawArrow(c, stuff.endX, stuff.endY, Math.atan2(-this.deltaY, -this.deltaX));
};

StartLink.prototype.getMidpoint = function() {
	var stuff = this.getEndPoints();
	return { x: (stuff.startX + stuff.endX) / 2, y: (stuff.startY + stuff.endY) / 2 };
};

StartLink.prototype.drawArcOnly = function(c) {
	var stuff = this.getEndPoints();
	c.beginPath();
	c.moveTo(stuff.startX, stuff.startY);
	c.lineTo(stuff.endX, stuff.endY);
	c.stroke();
};

StartLink.prototype.containsPoint = function(x, y) {
	var stuff = this.getEndPoints();
	var dx = stuff.endX - stuff.startX;
	var dy = stuff.endY - stuff.startY;
	var length = Math.sqrt(dx*dx + dy*dy);
	var percent = (dx * (x - stuff.startX) + dy * (y - stuff.startY)) / (length * length);
	var distance = (dx * (y - stuff.startY) - dy * (x - stuff.startX)) / length;
	return (percent > 0 && percent < 1 && Math.abs(distance) < hitTargetPadding);
};

function TemporaryLink(from, to) {
	this.from = from;
	this.to = to;
}

TemporaryLink.prototype.draw = function(c) {
	// draw the line
	c.beginPath();
	c.moveTo(this.to.x, this.to.y);
	c.lineTo(this.from.x, this.from.y);
	c.stroke();

	// draw the head of the arrow
	drawArrow(c, this.to.x, this.to.y, Math.atan2(this.to.y - this.from.y, this.to.x - this.from.x));
};

// draw using this instead of a canvas and call toLaTeX() afterward
function ExportAsLaTeX(bounds, monochrome) {
	this.bounds = bounds;
	this._points = [];
	this._texData = '';
	this._scale = 0.1; // to convert pixels to document space (TikZ breaks if the numbers get too big, above 500?)
	this._monochrome = !!monochrome;

	// Convert a CSS colour value to a TikZ-compatible colour string.
	// In monochrome mode always returns 'black'.
	// Named colours ('black', 'white') pass through. CSS hex (#rrggbb) is
	// converted to TikZ inline RGB syntax which requires no extra packages.
	this._tikzColor = function(css) {
		if(this._monochrome) return 'black';
		if(!css || css === 'black') return 'black';
		if(css === 'white') return 'white';
		var m = css.match(/^#([0-9a-fA-F]{6})$/);
		if(m) {
			var h = m[1];
			var r = parseInt(h.slice(0,2), 16);
			var g = parseInt(h.slice(2,4), 16);
			var b = parseInt(h.slice(4,6), 16);
			return 'color={rgb,255:red,' + r + ';green,' + g + ';blue,' + b + '}';
		}
		return 'black'; // fallback for anything unrecognised
	};

	// Fix 1: line width — convert canvas lineWidth (px) to pt at the
	// compound scale (×0.1 _scale, ×0.2 tikzpicture) so strokes match screen.
	// canvas px → TikZ coord unit is ×0.02; then ×2.845 pt/unit → ×0.0569.
	// A small boost keeps lines visible: multiply by 1.8 for visual match.
	this._tikzLineWidth = function() { return '1.2pt'; };

	// Complete mapping of Latin-1 Supplement (U+00A0-U+00FF) to LaTeX equivalents.
	// This covers every non-ASCII character in the ANSI/Latin-1 range.
	// Characters above U+00FF fall back to ? substitution in escapedText.
	this._unicodeMap = {
		0x00A0:'~',
		0x00A1:'!`',
		0x00A2:'\\textcent{}',
		0x00A3:'\\pounds{}',
		0x00A4:'\\textcurrency{}',
		0x00A5:'\\textyen{}',
		0x00A6:'\\textbrokenbar{}',
		0x00A7:'\\textsection{}',
		0x00A8:'\\"{}',
		0x00A9:'\\textcopyright{}',
		0x00AA:'\\textordfeminine{}',
		0x00AB:'\\guillemotleft{}',
		0x00AC:'\\ensuremath{\\neg}',
		0x00AD:'\\-',
		0x00AE:'\\textregistered{}',
		0x00AF:'\\={}',
		0x00B0:'\\textdegree{}',
		0x00B1:'\\ensuremath{\\pm}',
		0x00B2:'\\textsuperscript{2}',
		0x00B3:'\\textsuperscript{3}',
		0x00B4:"\\'{}",
		0x00B5:'\\ensuremath{\\mu}',
		0x00B6:'\\textparagraph{}',
		0x00B7:'\\ensuremath{\\cdot}',
		0x00B8:'\\c{}',
		0x00B9:'\\textsuperscript{1}',
		0x00BA:'\\textordmasculine{}',
		0x00BB:'\\guillemotright{}',
		0x00BC:'\\textonequarter{}',
		0x00BD:'\\textonehalf{}',
		0x00BE:'\\textthreequarters{}',
		0x00BF:'?`',
		0x00C0:'\\`{A}',  0x00C1:"\\'{ A}",  0x00C2:'\\^{A}',  0x00C3:'\\~{A}',
		0x00C4:'\\"{A}',  0x00C5:'\\AA{}',   0x00C6:'\\AE{}',   0x00C7:'\\c{C}',
		0x00C8:'\\`{E}',  0x00C9:"\\'{ E}",  0x00CA:'\\^{E}',  0x00CB:'\\"{E}',
		0x00CC:'\\`{I}',  0x00CD:"\\'{ I}",  0x00CE:'\\^{I}',  0x00CF:'\\"{I}',
		0x00D0:'\\DH{}',  0x00D1:'\\~{N}',   0x00D2:'\\`{O}',  0x00D3:"\\'{ O}",
		0x00D4:'\\^{O}',  0x00D5:'\\~{O}',   0x00D6:'\\"{O}',  0x00D7:'\\ensuremath{\\times}',
		0x00D8:'\\O{}',   0x00D9:'\\`{U}',   0x00DA:"\\'{ U}",  0x00DB:'\\^{U}',
		0x00DC:'\\"{U}',  0x00DD:"\\'{ Y}",  0x00DE:'\\TH{}',  0x00DF:'\\ss{}',
		0x00E0:'\\`{a}',  0x00E1:"\\'{ a}",  0x00E2:'\\^{a}',  0x00E3:'\\~{a}',
		0x00E4:'\\"{a}',  0x00E5:'\\aa{}',   0x00E6:'\\ae{}',   0x00E7:'\\c{c}',
		0x00E8:'\\`{e}',  0x00E9:"\\'{ e}",  0x00EA:'\\^{e}',  0x00EB:'\\"{e}',
		0x00EC:'\\`{\\i}', 0x00ED:"\\'{ \\i}", 0x00EE:'\\^{\\i}', 0x00EF:'\\"{\\i}',
		0x00F0:'\\dh{}',  0x00F1:'\\~{n}',   0x00F2:'\\`{o}',  0x00F3:"\\'{ o}",
		0x00F4:'\\^{o}',  0x00F5:'\\~{o}',   0x00F6:'\\"{o}',  0x00F7:'\\ensuremath{\\div}',
		0x00F8:'\\o{}',   0x00F9:'\\`{u}',   0x00FA:"\\'{ u}",  0x00FB:'\\^{u}',
		0x00FC:'\\"{u}',  0x00FD:"\\'{ y}",  0x00FE:'\\th{}',  0x00FF:'\\"{y}',
		// Greek uppercase (U+0391-U+03A9)
		0x0391:'\\ensuremath{A}',          0x0392:'\\ensuremath{B}',
		0x0393:'\\ensuremath{\\Gamma}',   0x0394:'\\ensuremath{\\Delta}',
		0x0395:'\\ensuremath{E}',           0x0396:'\\ensuremath{Z}',
		0x0397:'\\ensuremath{H}',           0x0398:'\\ensuremath{\\Theta}',
		0x0399:'\\ensuremath{I}',           0x039A:'\\ensuremath{K}',
		0x039B:'\\ensuremath{\\Lambda}',  0x039C:'\\ensuremath{M}',
		0x039D:'\\ensuremath{N}',           0x039E:'\\ensuremath{\\Xi}',
		0x039F:'\\ensuremath{O}',           0x03A0:'\\ensuremath{\\Pi}',
		0x03A1:'\\ensuremath{P}',           0x03A3:'\\ensuremath{\\Sigma}',
		0x03A4:'\\ensuremath{T}',           0x03A5:'\\ensuremath{\\Upsilon}',
		0x03A6:'\\ensuremath{\\Phi}',      0x03A7:'\\ensuremath{X}',
		0x03A8:'\\ensuremath{\\Psi}',      0x03A9:'\\ensuremath{\\Omega}',
		// Greek lowercase (U+03B1-U+03C9)
		0x03B1:'\\ensuremath{\\alpha}',    0x03B2:'\\ensuremath{\\beta}',
		0x03B3:'\\ensuremath{\\gamma}',    0x03B4:'\\ensuremath{\\delta}',
		0x03B5:'\\ensuremath{\\varepsilon}',0x03B6:'\\ensuremath{\\zeta}',
		0x03B7:'\\ensuremath{\\eta}',      0x03B8:'\\ensuremath{\\theta}',
		0x03B9:'\\ensuremath{\\iota}',     0x03BA:'\\ensuremath{\\kappa}',
		0x03BB:'\\ensuremath{\\lambda}',   0x03BC:'\\ensuremath{\\mu}',
		0x03BD:'\\ensuremath{\\nu}',       0x03BE:'\\ensuremath{\\xi}',
		0x03BF:'\\ensuremath{o}',           0x03C0:'\\ensuremath{\\pi}',
		0x03C1:'\\ensuremath{\\rho}',      0x03C2:'\\ensuremath{\\varsigma}',
		0x03C3:'\\ensuremath{\\sigma}',    0x03C4:'\\ensuremath{\\tau}',
		0x03C5:'\\ensuremath{\\upsilon}',  0x03C6:'\\ensuremath{\\varphi}',
		0x03C7:'\\ensuremath{\\chi}',      0x03C8:'\\ensuremath{\\psi}',
		0x03C9:'\\ensuremath{\\omega}',
		// Unicode subscript digits U+2080-U+2089
		0x2080:'\\textsubscript{0}', 0x2081:'\\textsubscript{1}',
		0x2082:'\\textsubscript{2}', 0x2083:'\\textsubscript{3}',
		0x2084:'\\textsubscript{4}', 0x2085:'\\textsubscript{5}',
		0x2086:'\\textsubscript{6}', 0x2087:'\\textsubscript{7}',
		0x2088:'\\textsubscript{8}', 0x2089:'\\textsubscript{9}',
		// Unicode superscript digits U+2070, U+00B9, U+00B2, U+00B3, U+2074-U+2079
		0x2070:'\\textsuperscript{0}', 0x2074:'\\textsuperscript{4}',
		0x2075:'\\textsuperscript{5}', 0x2076:'\\textsuperscript{6}',
		0x2077:'\\textsuperscript{7}', 0x2078:'\\textsuperscript{8}',
		0x2079:'\\textsuperscript{9}',
		// Mathematical operators and symbols
		0x2200:'\\ensuremath{\\forall}',    0x2203:'\\ensuremath{\\exists}',
		0x2204:'\\ensuremath{\\nexists}',   0x2205:'\\ensuremath{\\emptyset}',
		0x2207:'\\ensuremath{\\nabla}',     0x2208:'\\ensuremath{\\in}',
		0x2209:'\\ensuremath{\\notin}',     0x220B:'\\ensuremath{\\ni}',
		0x220F:'\\ensuremath{\\prod}',      0x2211:'\\ensuremath{\\sum}',
		0x2212:'\\ensuremath{-}',            0x2213:'\\ensuremath{\\mp}',
		0x2215:'\\ensuremath{/}',            0x2217:'\\ensuremath{*}',
		0x221A:'\\ensuremath{\\sqrt{}}',    0x221B:'\\ensuremath{\\sqrt[3]{}}',
		0x221E:'\\ensuremath{\\infty}',     0x2202:'\\ensuremath{\\partial}',
		0x2220:'\\ensuremath{\\angle}',     0x2221:'\\ensuremath{\\measuredangle}',
		0x2225:'\\ensuremath{\\parallel}',  0x2226:'\\ensuremath{\\nparallel}',
		0x2227:'\\ensuremath{\\wedge}',     0x2228:'\\ensuremath{\\vee}',
		0x2229:'\\ensuremath{\\cap}',       0x222A:'\\ensuremath{\\cup}',
		0x222B:'\\ensuremath{\\int}',       0x222C:'\\ensuremath{\\iint}',
		0x2236:'\\ensuremath{:}',            0x2237:'\\ensuremath{::}',
		0x223C:'\\ensuremath{\\sim}',       0x2243:'\\ensuremath{\\simeq}',
		0x2245:'\\ensuremath{\\cong}',      0x2248:'\\ensuremath{\\approx}',
		0x224D:'\\ensuremath{\\asymp}',     0x2250:'\\ensuremath{\\doteq}',
		0x2260:'\\ensuremath{\\neq}',       0x2261:'\\ensuremath{\\equiv}',
		0x2262:'\\ensuremath{\\not\\equiv}',0x2264:'\\ensuremath{\\leq}',
		0x2265:'\\ensuremath{\\geq}',       0x226A:'\\ensuremath{\\ll}',
		0x226B:'\\ensuremath{\\gg}',        0x2282:'\\ensuremath{\\subset}',
		0x2283:'\\ensuremath{\\supset}',    0x2286:'\\ensuremath{\\subseteq}',
		0x2287:'\\ensuremath{\\supseteq}',  0x22A5:'\\ensuremath{\\perp}',
		0x22C5:'\\ensuremath{\\cdot}',      0x22EF:'\\ensuremath{\\cdots}',
		// Arrows
		0x2190:'\\ensuremath{\\leftarrow}',  0x2191:'\\ensuremath{\\uparrow}',
		0x2192:'\\ensuremath{\\rightarrow}', 0x2193:'\\ensuremath{\\downarrow}',
		0x2194:'\\ensuremath{\\leftrightarrow}',0x2195:'\\ensuremath{\\updownarrow}',
		0x21D0:'\\ensuremath{\\Leftarrow}',  0x21D1:'\\ensuremath{\\Uparrow}',
		0x21D2:'\\ensuremath{\\Rightarrow}', 0x21D3:'\\ensuremath{\\Downarrow}',
		0x21D4:'\\ensuremath{\\Leftrightarrow}',
		0x21A6:'\\ensuremath{\\mapsto}',     0x21A9:'\\ensuremath{\\hookleftarrow}',
		// Miscellaneous technical
		0x2126:'\\ensuremath{\\Omega}',      0x212B:'\\AA{}',
		0x2032:"\\ensuremath{'}",            0x2033:"\\ensuremath{''}",
		0x2026:'\\ldots{}',
		// Typography
		0x2013:'--',  0x2014:'---',
		0x201C:'``',  0x201D:"''",  0x2018:'`',  0x2019:"'",
	};

	// Scan all node and link labels; collect Unicode chars and build preamble declarations.
	this._buildUnicodeDeclarations = function() {
		var texts = [];
		for(var i = 0; i < nodes.length; i++) texts.push(nodes[i].text || '');
		for(var i = 0; i < links.length; i++) {
			texts.push(links[i].text || '');
			if(links[i]._id) {
				var lp = linkProperties.get(links[i]._id) || {};
				if(lp.guard)  texts.push(lp.guard);
				if(lp.action) texts.push(lp.action);
				if(lp.output) texts.push(lp.output);
			}
		}
		var seen = {};
		var decls = '';
		var allText = texts.join('');
		for(var j = 0; j < allText.length; j++) {
			var cp = allText.codePointAt(j);
			if(cp > 0x7F && cp <= 0xFF && !seen[cp]) {
				seen[cp] = true;
				var hex = cp.toString(16).toUpperCase().padStart(4, '0');
				var mapped = this._unicodeMap[cp];
				if(mapped) {
					decls += '\\DeclareUnicodeCharacter{' + hex + '}{' + mapped + '}\n';
				}
				// Unknown chars: fallthrough to the ?-replacement in escapedText
			}
		}
		return decls;
	};

	this.toLaTeX = function() {
		var uDecls = this._buildUnicodeDeclarations();
		return '\\documentclass[10pt]{article}\n' +
			'\\usepackage{tikz}\n' +
			'\\usepackage[utf8]{inputenc}\n' +
			'\\renewcommand{\\familydefault}{\\sfdefault}\n' +
			(uDecls ? uDecls : '') +
			'\n' +
			'\\begin{document}\n' +
			'\n' +
			'\\begin{center}\n' +
			'\\resizebox{\\textwidth}{!}{%\n' +
			'\\begin{tikzpicture}[scale=0.2]\n' +
			'\\tikzstyle{every node}+=[inner sep=0pt]\n' +
			this._texData +
			'\\end{tikzpicture}%\n' +
			'}\n' +
			'\\end{center}\n' +
			'\n' +
			'\\end{document}\n';
	};

	this.beginPath = function() {
		this._points = [];
	};
	this.arc = function(x, y, radius, startAngle, endAngle, isReversed) {
		x -= this.bounds[0];
		y -= this.bounds[1];
		x *= this._scale;
		y *= this._scale;
		radius *= this._scale;
		var col = this._tikzColor(this.strokeStyle);
		var lw  = this._tikzLineWidth();
		if(endAngle - startAngle == Math.PI * 2) {
			this._texData += '\\draw [' + col + ',line width=' + lw + '] (' + fixed(x, 3) + ',' + fixed(-y, 3) + ') circle (' + fixed(radius, 3) + ');\n';
		} else {
			if(isReversed) {
				var temp = startAngle;
				startAngle = endAngle;
				endAngle = temp;
			}
			if(endAngle < startAngle) {
				endAngle += Math.PI * 2;
			}
			// TikZ needs the angles to be in between -2pi and 2pi or it breaks
			if(Math.min(startAngle, endAngle) < -2*Math.PI) {
				startAngle += 2*Math.PI;
				endAngle += 2*Math.PI;
			} else if(Math.max(startAngle, endAngle) > 2*Math.PI) {
				startAngle -= 2*Math.PI;
				endAngle -= 2*Math.PI;
			}
			startAngle = -startAngle;
			endAngle = -endAngle;
			this._texData += '\\draw [' + col + ',line width=' + lw + '] (' + fixed(x + radius * Math.cos(startAngle), 3) + ',' + fixed(-y + radius * Math.sin(startAngle), 3) + ') arc (' + fixed(startAngle * 180 / Math.PI, 5) + ':' + fixed(endAngle * 180 / Math.PI, 5) + ':' + fixed(radius, 3) + ');\n';
		}
	};
	this.moveTo = this.lineTo = function(x, y) {
		x -= this.bounds[0];
		y -= this.bounds[1];
		x *= this._scale;
		y *= this._scale;
		this._points.push({ 'x': x, 'y': y });
	};
	this.stroke = function() {
		if(this._points.length == 0) return;
		this._texData += '\\draw [' + this._tikzColor(this.strokeStyle) + ',line width=' + this._tikzLineWidth() + ']';
		for(var i = 0; i < this._points.length; i++) {
			var p = this._points[i];
			this._texData += (i > 0 ? ' --' : '') + ' (' + fixed(p.x, 2) + ',' + fixed(-p.y, 2) + ')';
		}
		this._texData += ';\n';
	};
	this.fill = function() {
		if(this._points.length == 0) return;
		this._texData += '\\fill [' + this._tikzColor(this.strokeStyle) + ']';
		for(var i = 0; i < this._points.length; i++) {
			var p = this._points[i];
			this._texData += (i > 0 ? ' --' : '') + ' (' + fixed(p.x, 2) + ',' + fixed(-p.y, 2) + ')';
		}
		this._texData += ';\n';
	};
	this.measureText = function(text) {
		var c = canvas.getContext('2d');
		c.font = seamFont;
		return c.measureText(text);
	};
	this.advancedFillText = function(text, originalText, x, y, angleOrNull) {
		x -= this.bounds[0];
		y -= this.bounds[1];
		if(text.replace(' ', '').length > 0) {
			var nodeParams = '';
			// x and y start off as the center of the text, but will be moved to one side of the box when angleOrNull != null
			if(angleOrNull != null) {
				var width = this.measureText(text).width;
				var dx = Math.cos(angleOrNull);
				var dy = Math.sin(angleOrNull);
				if(Math.abs(dx) > Math.abs(dy)) {
					if(dx > 0) nodeParams = '[right] ', x -= width / 2;
					else nodeParams = '[left] ', x += width / 2;
				} else {
					if(dy > 0) nodeParams = '[below] ', y -= 10;
					else nodeParams = '[above] ', y += 10;
				}
			}
			x *= this._scale;
			y *= this._scale;
			// Fix 2: label colour is always black (labels are dark on a light node)
			// Fix 3: \small reduces font ~17% relative to 10pt base
			// Fix 4: \sffamily\upshape gives sans-serif upright (no math mode)
			// Replace Unicode sub/superscripts before other escaping
			var _subMap = {'₀':'\\textsubscript{0}','₁':'\\textsubscript{1}','₂':'\\textsubscript{2}','₃':'\\textsubscript{3}','₄':'\\textsubscript{4}','₅':'\\textsubscript{5}','₆':'\\textsubscript{6}','₇':'\\textsubscript{7}','₈':'\\textsubscript{8}','₉':'\\textsubscript{9}'};
			var _supMap = {'⁰':'\\textsuperscript{0}','¹':'\\textsuperscript{1}','²':'\\textsuperscript{2}','³':'\\textsuperscript{3}','⁴':'\\textsuperscript{4}','⁵':'\\textsuperscript{5}','⁶':'\\textsuperscript{6}','⁷':'\\textsuperscript{7}','⁸':'\\textsuperscript{8}','⁹':'\\textsuperscript{9}'};
			var escapedText = originalText
				.replace(/[₀₁₂₃₄₅₆₇₈₉]/g, function(c){return _subMap[c]||c;})
				.replace(/[⁰¹²³⁴⁵⁶⁷⁸⁹]/g, function(c){return _supMap[c]||c;})
				.replace(/[^\x00-\xFF]/g, (function(self) { return function(c) {
					var mapped = self._unicodeMap[c.codePointAt(0)];
					return mapped !== undefined ? mapped : '?';
				}; })(this)) // substitute from map, fallback ?
				.replace(/\\/g, '\\textbackslash{}')
				.replace(/{/g, '\\{')
				.replace(/}/g, '\\}')
				.replace(/_/g, '\\_')
				.replace(/\^/g, '\\^{}')
				.replace(/&/g, '\\&')
				.replace(/%/g, '\\%')
				.replace(/\$/g, '\\$')
				.replace(/#/g, '\\#')
				.replace(/~/g, '\\textasciitilde{}')
				.replace(/</g, '\\textless{}')
				.replace(/>/g, '\\textgreater{}')
				.replace(/"/g, "'");
			this._texData += '\\draw [black] (' + fixed(x, 2) + ',' + fixed(-y, 2) + ') node ' + nodeParams + '{\\fontsize{5.99}{7.2}\\selectfont\\sffamily\\upshape ' + escapedText + '};\n';
		}
	};

	this.translate = this.save = this.restore = this.clearRect = this.scale = function(){};
}

// draw using this instead of a canvas and call toSVG() afterward
function ExportAsSVG(bounds) {
	this.width = bounds[2] - bounds[0];
	this.height = bounds[3] - bounds[1];
	this.bounds = bounds;
	this.fillStyle = 'black';
	this.strokeStyle = 'black';
	this.lineWidth = 1;
	this.font = '12px Arial, sans-serif';
	this._points = [];
	this._svgData = '';
	this._transX = 0;
	this._transY = 0;

	this.toSVG = function() {
		var data = '<?xml version="1.0" standalone="no"?>\n';
		data += '<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">\n\n';
		data += '<svg width="' + this.width + '" height="' + this.height + '" version="1.1" xmlns="http://www.w3.org/2000/svg">\n';
		data += this._svgData;
		data += '</svg>\n';
		return data;
	};

	this.beginPath = function() {
		this._points = [];
	};
	this.arc = function(x, y, radius, startAngle, endAngle, isReversed) {
		x -= this.bounds[0];
		y -= this.bounds[1];
		x += this._transX;
		y += this._transY;
		var style = 'stroke="' + this.strokeStyle + '" stroke-width="' + this.lineWidth + '" fill="none"';

		if(endAngle - startAngle == Math.PI * 2) {
			this._svgData += '\t<ellipse ' + style + ' cx="' + fixed(x, 3) + '" cy="' + fixed(y, 3) + '" rx="' + fixed(radius, 3) + '" ry="' + fixed(radius, 3) + '"/>\n';
		} else {
			if(isReversed) {
				var temp = startAngle;
				startAngle = endAngle;
				endAngle = temp;
			}

			if(endAngle < startAngle) {
				endAngle += Math.PI * 2;
			}

			var startX = x + radius * Math.cos(startAngle);
			var startY = y + radius * Math.sin(startAngle);
			var endX = x + radius * Math.cos(endAngle);
			var endY = y + radius * Math.sin(endAngle);
			var useGreaterThan180 = (Math.abs(endAngle - startAngle) > Math.PI);
			var goInPositiveDirection = 1;

			this._svgData += '\t<path ' + style + ' d="';
			this._svgData += 'M ' + fixed(startX, 3) + ',' + fixed(startY, 3) + ' '; // startPoint(startX, startY)
			this._svgData += 'A ' + fixed(radius, 3) + ',' + fixed(radius, 3) + ' '; // radii(radius, radius)
			this._svgData += '0 '; // value of 0 means perfect circle, others mean ellipse
			this._svgData += +useGreaterThan180 + ' ';
			this._svgData += +goInPositiveDirection + ' ';
			this._svgData += fixed(endX, 3) + ',' + fixed(endY, 3); // endPoint(endX, endY)
			this._svgData += '"/>\n';
		}
	};
	this.moveTo = this.lineTo = function(x, y) {
		x -= this.bounds[0];
		y -= this.bounds[1];
		x += this._transX;
		y += this._transY;
		this._points.push({ 'x': x, 'y': y });
	};
	this.stroke = function() {
		if(this._points.length == 0) return;
		this._svgData += '\t<polygon stroke="' + this.strokeStyle + '" stroke-width="' + this.lineWidth + '" points="';
		for(var i = 0; i < this._points.length; i++) {
			this._svgData += (i > 0 ? ' ' : '') + fixed(this._points[i].x, 3) + ',' + fixed(this._points[i].y, 3);
		}
		this._svgData += '"/>\n';
	};
	this.fill = function() {
		if(this._points.length == 0) return;
		this._svgData += '\t<polygon fill="' + this.fillStyle + '" stroke-width="' + this.lineWidth + '" points="';
		for(var i = 0; i < this._points.length; i++) {
			this._svgData += (i > 0 ? ' ' : '') + fixed(this._points[i].x, 3) + ',' + fixed(this._points[i].y, 3);
		}
		this._svgData += '"/>\n';
	};
	this.measureText = function(text) {
		var c = canvas.getContext('2d');
		c.font = seamFont;
		return c.measureText(text);
	};
	this.fillText = function(text, x, y) {
		x -= this.bounds[0];
		y -= this.bounds[1];
		x += this._transX;
		y += this._transY;
		if(text.replace(' ', '').length > 0) {
			this._svgData += '\t<text x="' + fixed(x, 3) + '" y="' + fixed(y, 3) + '" font-family="Times New Roman" font-size="20">' + textToXML(text) + '</text>\n';
		}
	};
	this.translate = function(x, y) {
		this._transX = x;
		this._transY = y;
	};
	this.scale = function() {}; // no-op: viewport reset to k=1 before export

	this.advancedFillText = function(text, originalText, x, y, angleOrNull) {
		x -= this.bounds[0];
		y -= this.bounds[1];
		x += this._transX;
		y += this._transY;
		if(text.replace(' ', '').length === 0) return;
		// Parse font size from seamFont (e.g. "12px system-ui, ...")
		var fontSize = 12;
		if(typeof seamFont === 'string') {
			var fm = seamFont.match(/^(\d+(?:\.\d+)?)px/);
			if(fm) fontSize = parseFloat(fm[1]);
		}
		// Use seamActiveLabelColor for the fill, falling back to strokeStyle
		var fill = (typeof seamActiveLabelColor === 'string' && seamActiveLabelColor)
			? seamActiveLabelColor : this.strokeStyle;
		this._svgData += '\t<text x="' + fixed(x, 3) + '" y="' + fixed(y + fontSize * 0.35, 3) + '"' +
			' font-family="sans-serif" font-size="' + fontSize + '"' +
			' text-anchor="middle" fill="' + fill + '">' + textToXML(text) + '</text>\n';
	};

	this.save = this.restore = this.clearRect = function(){};
}

var greekLetterNames = [ 'Alpha', 'Beta', 'Gamma', 'Delta', 'Epsilon', 'Zeta', 'Eta', 'Theta', 'Iota', 'Kappa', 'Lambda', 'Mu', 'Nu', 'Xi', 'Omicron', 'Pi', 'Rho', 'Sigma', 'Tau', 'Upsilon', 'Phi', 'Chi', 'Psi', 'Omega', 'emptyset', 'rightarrow', 'leftarrow'];

function convertLatexShortcuts(text) {
	// html greek characters
	for(var i = 0; i < greekLetterNames.length; i++) {
		var name = greekLetterNames[i];
		if (name == "emptyset") {
			text = text.replace(new RegExp('\\\\' + name, 'g'), String.fromCharCode(8709));
			continue;
		}
		if (name == "rightarrow") {
			text = text.replace(new RegExp('\\\\' + name, 'g'), String.fromCharCode(8594));
			continue;
		}
		if (name == "leftarrow") {
			text = text.replace(new RegExp('\\\\' + name, 'g'), String.fromCharCode(8592));
			continue;
		}
		text = text.replace(new RegExp('\\\\' + name, 'g'), String.fromCharCode(913 + i + (i > 16)));
		text = text.replace(new RegExp('\\\\' + name.toLowerCase(), 'g'), String.fromCharCode(945 + i + (i > 16)));
	}

	// subscripts
	for(var i = 0; i < 10; i++) {
		text = text.replace(new RegExp('_' + i, 'g'), String.fromCharCode(8320 + i));
	}

	return text;
}

function textToXML(text) {
	text = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
	var result = '';
	for(var i = 0; i < text.length; i++) {
		var c = text.charCodeAt(i);
		if(c >= 0x20 && c <= 0x7E) {
			result += text[i];
		} else {
			result += '&#' + c + ';';
		}
	}
	return result;
}

function drawArrow(c, x, y, angle) {
	var dx = Math.cos(angle);
	var dy = Math.sin(angle);
	c.beginPath();
	c.moveTo(x, y);
	c.lineTo(x - 10 * dx + 6 * dy, y - 10 * dy - 6 * dx);
	c.lineTo(x - 10 * dx - 6 * dy, y - 10 * dy + 6 * dx);
	c.fill();
}

function canvasHasFocus() {
	return (document.activeElement || document.body) == document.body;
}

// Caret helpers for multi-line node labels
function caretToLineCol(text, pos) {
	var lines = text.split('\n');
	var remaining = pos;
	for(var i = 0; i < lines.length; i++) {
		if(remaining <= lines[i].length) return { line: i, col: remaining };
		remaining -= lines[i].length + 1;
	}
	var last = lines.length - 1;
	return { line: last, col: lines[last].length };
}

function lineColToPos(text, targetLine, targetCol) {
	var lines = text.split('\n');
	var line = Math.min(targetLine, lines.length - 1);
	var pos = 0;
	for(var i = 0; i < line; i++) pos += lines[i].length + 1;
	return pos + Math.min(targetCol, lines[line].length);
}

// Multi-line rendering for node labels (angleOrNull === null).
// cx/cy is the centre of the node.
function _drawTextMultiline(c, text, originalText, cx, cy, isSelected) {
	var lines = text.split('\n');
	var maxWidth = 0;
	for(var i = 0; i < lines.length; i++) maxWidth = Math.max(maxWidth, c.measureText(lines[i]).width);
	var totalHeight = lines.length * seamLineHeight;

	if('advancedFillText' in c) {
		// LaTeX export: render each line with y offset
		var startYex = cy - (lines.length - 1) * seamLineHeight / 2;
		for(var i = 0; i < lines.length; i++)
			c.advancedFillText(lines[i], lines[i], cx, startYex + i * seamLineHeight, null);
		return;
	}

	cx = Math.round(cx); cy = Math.round(cy);
	var editing = isSelected && editingLabel && canvasHasFocus() && document.hasFocus();
	if(editing && _suppressEditBox) return; // deferred to top-most overlay pass

	if(editing) {
		var pad = 7;
		var bw = Math.max(maxWidth, 20) + pad * 2;
		var bh = totalHeight + pad * 2;
		var bx = cx - Math.max(maxWidth, 20) / 2 - pad;
		var by = cy - totalHeight / 2 - pad;
		var sf = c.fillStyle, ss = c.strokeStyle, slw = c.lineWidth;
		c.fillStyle = seamEditBg; c.strokeStyle = seamEditBorder; c.lineWidth = 1;
		c.beginPath();
		if(c.roundRect) c.roundRect(bx, by, bw, bh, 4); else c.rect(bx, by, bw, bh);
		c.fill(); c.stroke();
		c.fillStyle = sf; c.strokeStyle = ss; c.lineWidth = slw;
	}

	var savedFill = c.fillStyle;
	c.fillStyle = seamActiveLabelColor || savedFill;
	var startY = cy - (lines.length - 1) * seamLineHeight / 2;
	for(var i = 0; i < lines.length; i++) {
		var lw = c.measureText(lines[i]).width;
		c.fillText(lines[i], cx - lw / 2, startY + i * seamLineHeight + 5);
	}
	c.fillStyle = savedFill;

	if(editing && caretVisible) {
		var lc = caretToLineCol(originalText, Math.min(caretPosition, originalText.length));
		var cli = Math.min(lc.line, lines.length - 1);
		var clt = lines[cli] || '';
		var col = Math.min(lc.col, clt.length);
		var caretX = cx - c.measureText(clt).width / 2 + c.measureText(clt.substring(0, col)).width;
		var caretY = startY + cli * seamLineHeight;
		c.beginPath();
		c.moveTo(caretX, caretY - 7); c.lineTo(caretX, caretY + 9);
		c.stroke();
	}
}

function drawText(c, originalText, x, y, angleOrNull, isSelected) {
	var rawText = convertLatexShortcuts(originalText);
	c.font = seamFont;

	// Node labels: delegate to multi-line renderer (no angle offset)
	if(angleOrNull === null) {
		_drawTextMultiline(c, rawText, originalText, x, y, isSelected);
		return;
	}

	// Link labels: multi-line, with Wallace angle-offset adjusted for block height
	var lines = rawText.split('\n');
	var maxWidth = 0;
	for(var i = 0; i < lines.length; i++) maxWidth = Math.max(maxWidth, c.measureText(lines[i]).width);
	var totalHeight = lines.length * seamLineHeight;

	var cos = Math.cos(angleOrNull);
	var sin = Math.sin(angleOrNull);
	// Both overrides are continuous ±1 floats animated by Link.draw.
	// Used as direct multipliers so both axes tween smoothly.
	var sinForSide = (_drawTextSinOverride !== null) ? _drawTextSinOverride : (sin >= 0 ? 1 : -1);
	var cosForSide = (_drawTextCosOverride !== null) ? _drawTextCosOverride : (cos >= 0 ? 1 : -1);
	var cornerPointX = (maxWidth / 2 + 5) * cosForSide;  // continuous, not sign-only
	var cornerPointY = (totalHeight / 2 + 5) * sinForSide; // continuous, not sign-only
	var slide = sin * Math.pow(Math.abs(sin), 40) * cornerPointX - cos * Math.pow(Math.abs(cos), 10) * cornerPointY;
	// cx/cy: centre of the text block in canvas space
	var cx = x + cornerPointX - sin * slide;
	var cy = y + cornerPointY + cos * slide;

	if('advancedFillText' in c) {
		// Export renderers: emit each line
		for(var i = 0; i < lines.length; i++) {
			var lineY = cy - (lines.length - 1) * seamLineHeight / 2 + i * seamLineHeight;
			c.advancedFillText(lines[i], lines[i], cx, lineY, angleOrNull);
		}
	} else {
		cx = Math.round(cx); cy = Math.round(cy);
		var editing = isSelected && editingLabel && canvasHasFocus() && document.hasFocus();

		if(editing) {
			// Axis-aligned edit rect (no rotation — labels always face the user)
			var pad = 7;
			var bw = Math.max(maxWidth, 20) + pad * 2;
			var bh = totalHeight + pad * 2;
			var sf = c.fillStyle, ss = c.strokeStyle, slw = c.lineWidth;
			c.fillStyle = seamEditBg; c.strokeStyle = seamEditBorder; c.lineWidth = 1;
			c.beginPath();
			if(c.roundRect) c.roundRect(cx - bw / 2, cy - bh / 2, bw, bh, 4);
			else c.rect(cx - bw / 2, cy - bh / 2, bw, bh);
			c.fill(); c.stroke();
			c.fillStyle = sf; c.strokeStyle = ss; c.lineWidth = slw;
		}

		// Draw each line centred around cx
		var savedFill = c.fillStyle;
		c.fillStyle = seamActiveLabelColor || savedFill;
		var startY = cy - (lines.length - 1) * seamLineHeight / 2;
		for(var i = 0; i < lines.length; i++) {
			var lw = c.measureText(lines[i]).width;
			c.fillText(lines[i], cx - lw / 2, startY + i * seamLineHeight + 5);
		}
		c.fillStyle = savedFill;

		// Caret — drawn in rotated coordinate frame
		if(editing && caretVisible) {
			var lc = caretToLineCol(originalText, Math.min(caretPosition, originalText.length));
			var cli = Math.min(lc.line, lines.length - 1);
			var clt = lines[cli] || '';
			var col = Math.min(lc.col, clt.length);
			// Local x in the line (relative to line centre)
			var localX = -c.measureText(clt).width / 2 + c.measureText(clt.substring(0, col)).width;
			var localY = startY + cli * seamLineHeight - cy; // relative to cy
			// Map local to canvas via rotation
			var caretCanvasX = cx + localX * Math.cos(0) - localY * Math.sin(0); // angle=0: text isn't actually rotated
			var caretCanvasY = cy + localY;
			c.beginPath();
			c.moveTo(cx + localX, caretCanvasY - 7);
			c.lineTo(cx + localX, caretCanvasY + 9);
			c.stroke();
		}
	}
}

var caretTimer;
var caretVisible = true;
var caretPosition = 0;  // index into selectedObject.text

function resetCaret() {
	clearInterval(caretTimer);
	caretTimer = setInterval('caretVisible = !caretVisible; draw()', 500);
	caretVisible = true;
}

// Edit-mode label background — set by Lit shell _applyTheme
var seamEditBg     = 'rgba(248,250,252,0.95)';
var seamEditBorder = 'rgba(99,102,241,0.5)';
// Temporary-link animation
var _linkPulseRafId  = null;  // RAF id for pulsating opacity during drag
var _linkPulseT      = 0;     // time accumulator (ms)
var _linkPulseAlpha  = 1;     // current globalAlpha for TemporaryLink draw
var _linkPulseLast   = null;  // DOMHighResTimeStamp of last frame
var _snapBackRafId   = null;  // RAF id for snap-back animation
var _snapBackLink    = null;  // frozen TemporaryLink during snap-back
var _snapBackActive  = false; // true while snap-back is running

function _startLinkPulse() {
	if(_linkPulseRafId !== null) return;
	_linkPulseLast = null;
	function pulseFrame(now) {
		if(_linkPulseLast === null) _linkPulseLast = now;
		_linkPulseT += now - _linkPulseLast;
		_linkPulseLast = now;
		var sine = Math.sin(_linkPulseT * 0.006); // ~1 Hz
		_linkPulseAlpha = 0.35 + 0.65 * (sine * 0.5 + 0.5); // range 0.35..1
		if(currentLink instanceof TemporaryLink && !_snapBackActive) {
			draw();
			_linkPulseRafId = requestAnimationFrame(pulseFrame);
		} else {
			_linkPulseAlpha = 1;
			_linkPulseRafId = null;
		}
	}
	_linkPulseRafId = requestAnimationFrame(pulseFrame);
}

function _stopLinkPulse() {
	if(_linkPulseRafId !== null) {
		cancelAnimationFrame(_linkPulseRafId);
		_linkPulseRafId = null;
	}
	_linkPulseAlpha = 1;
	_linkPulseLast  = null;
}

function _startSnapBack(link) {
	_stopLinkPulse();
	_snapBackLink   = { from: { x: link.from.x, y: link.from.y },
	                    to:   { x: link.to.x,   y: link.to.y   } };
	_snapBackActive = true;
	currentLink     = null;
	var startX = _snapBackLink.to.x, startY = _snapBackLink.to.y;
	var endX   = _snapBackLink.from.x, endY = _snapBackLink.from.y;
	var startT = performance.now();
	var dur    = 520; // ms
	function snapFrame(now) {
		var t = Math.min((now - startT) / dur, 1);
		var ease = 1 - Math.pow(1 - t, 3); // ease-out cubic
		_snapBackLink.to.x = startX + (endX - startX) * ease;
		_snapBackLink.to.y = startY + (endY - startY) * ease;
		var alpha = 1 - ease * 0.85; // fades toward 0.15 then gone
		_linkPulseAlpha = alpha;
		draw();
		if(t < 1) {
			_snapBackRafId = requestAnimationFrame(snapFrame);
		} else {
			_snapBackActive = false;
			_snapBackLink   = null;
			_linkPulseAlpha = 1;
			_snapBackRafId  = null;
			draw();
		}
	}
	_snapBackRafId = requestAnimationFrame(snapFrame);
}

// Label editing: only active after stationary mouseup
var editingLabel       = false;
var _suppressEditBox   = false; // true during main pass; edit overlay drawn last
var _hasDragged    = false;
// Halo: node primed on exit; opacity animates in fast, out slow
var _haloNode      = null;
// Link hover: null or the link under the mouse pointer
var _hoveredLink   = null;
// Dot: separate wider-tolerance hover for midpoint dot animation
var _dotLink       = null;
var _dotOpacity    = 0;
var _dotTarget     = 0;
var _dotRafId      = null;

function _startDotAnim() {
	if(_dotRafId !== null) return;
	function dotFrame() {
		var step = (_dotTarget > _dotOpacity) ? 0.15 : 0.09;
		var diff = _dotTarget - _dotOpacity;
		if(Math.abs(diff) < 0.004) {
			_dotOpacity = _dotTarget;
			_dotRafId = null;
			if(_dotTarget === 0) _dotLink = null;
			draw();
			return;
		}
		_dotOpacity += (diff > 0 ? 1 : -1) * Math.min(step, Math.abs(diff));
		draw();
		_dotRafId = requestAnimationFrame(dotFrame);
	}
	_dotRafId = requestAnimationFrame(dotFrame);
}
var _haloVisible   = false;  // logical state (target)
var _haloOpacity   = 0;      // current rendered opacity 0..1
var _haloTarget    = 0;      // 0 or 1
var _haloRafId     = null;

function _startNodeGrow(node) {
	_growingNode = node;
	_growT = 0;
	if(_growRafId !== null) return;
	var lastT = null;
	function growFrame(t) {
		if(lastT === null) lastT = t;
		var dt = Math.min(t - lastT, 64); lastT = t;
		_growT += dt;
		if(_growT >= _growDur) {
			_growT = _growDur;
			_growingNode = null;
			_growRafId = null;
			draw(); return;
		}
		draw();
		_growRafId = requestAnimationFrame(growFrame);
	}
	_growRafId = requestAnimationFrame(growFrame);
}

function _startGroupHalo() {
	if(_groupHaloRafId !== null) return;
	var lastT = null;
	function groupHaloFrame(t) {
		if(_selectedNodes.length === 0) { _groupHaloRafId = null; return; }
		if(lastT === null) lastT = t;
		var dt = Math.min(t - lastT, 64); lastT = t;
		_groupHaloT += dt;
		_groupHaloAlpha = 0.45 + 0.45 * Math.sin(_groupHaloT * 0.007);
		draw();
		_groupHaloRafId = requestAnimationFrame(groupHaloFrame);
	}
	_groupHaloRafId = requestAnimationFrame(groupHaloFrame);
}

function _startHaloAnim() {
	if(_haloRafId !== null) return;
	function haloFrame() {
		var step = (_haloTarget > _haloOpacity) ? 0.18 : 0.07; // fast in, slow out
		var diff = _haloTarget - _haloOpacity;
		if(Math.abs(diff) < 0.005) {
			_haloOpacity = _haloTarget;
			_haloRafId = null;
			if(_haloTarget === 0) _haloNode = null; // fully faded — release node ref
			draw();
			return;
		}
		_haloOpacity += (diff > 0 ? 1 : -1) * Math.min(step, Math.abs(diff));
		draw();
		_haloRafId = requestAnimationFrame(haloFrame);
	}
	_haloRafId = requestAnimationFrame(haloFrame);
}
// Per-link guard/action properties (keyed by link._id)
var _linkIdCounter = 0;
var linkProperties = new Map(); // _id → {guard, action, output} — output added for xoluman's use (xolu transitions carry a distinct output value, separate from any action/side-effect text); Seam's own property editor and save/load only ever handled guard/action
// Per-link custom colour. Cycled by Shift+click.
var linkColors = new Map(); // _id → colour string

var canvas;
var nodeRadius = 30;
var nodes = [];
var links = [];

var cursorVisible = true;
var snapToPadding = 6; // pixels
var hitTargetPadding = 6; // pixels
var selectedObject = null; // either a Link or a Node
var currentLink = null; // a Link
var movingObject = false;
var originalClick;

// ── Viewport (pan + zoom) ─────────────────────────────────────────────────────
var viewport = { x: 0, y: 0, k: 1 };
// Font for all labels — writable by the Lit shell to sync with theme
var seamFont     = '12px system-ui, -apple-system, "Segoe UI", sans-serif';
var seamLineHeight = 16;  // px between baselines for multi-line labels
// Visual style globals — set by Lit shell _applyTheme
var seamAutoSize       = false;        // adapt node radius to label width
var seamAutoSizePad    = 14;           // inner padding (px) for auto-sized nodes
// Link label side-flip smoothing: set by Link.draw, read by drawText
var _drawTextSinOverride = null; // float set by Link.draw, read by drawText
var _drawTextCosOverride = null; // float set by Link.draw, read by drawText
var _labelFlipDur = 450;         // ms for one full side transition

function _smoothLabelAxis(link, targetVal, sideKey, fromKey, toKey, t0Key) {
	var target = targetVal >= 0 ? 1 : -1;
	if(link[sideKey] === undefined || link[sideKey] === null) {
		link[sideKey] = target; link[toKey] = target;
		link[fromKey] = target; link[t0Key]  = null;
		return target;
	}
	if(target !== link[toKey]) {
		link[fromKey] = link[sideKey];
		link[toKey]   = target;
		link[t0Key]   = performance.now();
		_scheduleLabelRaf();
	}
	return link[sideKey];
}

function _smoothLabelSide(link, targetSin) {
	return _smoothLabelAxis(link, targetSin, '_labelSide', '_labelFrom', '_labelTo', '_labelT0');
}

function _smoothLabelCos(link, targetCos) {
	return _smoothLabelAxis(link, targetCos, '_labelCosS', '_labelCosFrom', '_labelCosTo', '_labelCosT0');
}

var _labelFlipRafId = null;

function _scheduleLabelRaf() {
	if(_labelFlipRafId !== null) return;
	_labelFlipRafId = requestAnimationFrame(_labelFlipFrame);
}

function _labelFlipFrame(now) {
	_labelFlipRafId = null;
	var stillAnimating = false;
	for(var i = 0; i < links.length; i++) {
		var lnk = links[i];
		// Sin axis
		if(lnk._labelT0) {
			var t = Math.min((now - lnk._labelT0) / _labelFlipDur, 1);
			var ease = t * t * (3 - 2 * t);
			lnk._labelSide = lnk._labelFrom + (lnk._labelTo - lnk._labelFrom) * ease;
			if(t < 1) { stillAnimating = true; }
			else { lnk._labelSide = lnk._labelTo; lnk._labelT0 = null; }
		}
		// Cos axis
		if(lnk._labelCosT0) {
			var tc = Math.min((now - lnk._labelCosT0) / _labelFlipDur, 1);
			var easec = tc * tc * (3 - 2 * tc);
			lnk._labelCosS = lnk._labelCosFrom + (lnk._labelCosTo - lnk._labelCosFrom) * easec;
			if(tc < 1) { stillAnimating = true; }
			else { lnk._labelCosS = lnk._labelCosTo; lnk._labelCosT0 = null; }
		}
	}
	draw();
	if(stillAnimating) _labelFlipRafId = requestAnimationFrame(_labelFlipFrame);
}var seamNodeFill       = '#f8fafc';   // node interior fill
var seamAccentColor    = '#169B62';   // Irish green for accept states
var seamNodeStroke     = 1.5;         // circle stroke width
var seamLinkStroke     = 2.0;         // arc/line stroke width
var seamNodeStrokeColor = '#6366f1';  // node circle colour
var seamNodeLabelColor  = '#1e293b';  // text inside nodes
var seamLinkStrokeColor = '#818cf8';  // transition arc colour
var seamLinkLabelColor  = '#64748b';  // text on transitions
var seamActiveLabelColor = '#1e293b'; // set by drawUsing before each drawText
// Zoom inertia
var _zoomVelocity = 0;
var _zoomCursorX = 0;
var _zoomCursorY = 0;
var _zoomRafId   = null;
var panningCanvas = false;
var panStart = null;
// Add-linked-node mode (right-click halo)
var _addingLinkedNode = false;  // true when drag is in add-linked-node mode
var _addLinkedOrigin  = null;   // the source node for the new link
// Node grow animation
var _growingNode  = null;  // Node currently animating in
var _growT        = 0;     // elapsed ms
var _growDur      = 320;   // ms for full grow
var _growRafId    = null;

// Group (rubber-band) selection
var _selectedNodes  = [];     // nodes in the current group selection
var _rectSelecting  = false;  // true while right-drag rubber band is active
var _rectSelStart   = null;   // world coords of rubber-band anchor
var _rectSelCur     = null;   // world coords of current mouse pos
// Group-halo pulse (continuous sin wave, same pattern as link pulse)
var _groupHaloAlpha = 0.55;
var _groupHaloRafId = null;
var _groupHaloT     = 0;
var panViewStart = null;

function clearCanvas() {
	nodes = [];
	links = [];
	linkProperties.clear();
	linkColors.clear();
	_selectedNodes = [];
	_rectSelecting = false;
	_groupHaloRafId = null;
	_addingLinkedNode = false;
	_addLinkedOrigin  = null;
	_growingNode = null;
	localStorage.removeItem('fsm');
	viewport = { x: 0, y: 0, k: 1 };
	var context = canvas.getContext('2d');
	context.clearRect(0, 0, canvas.width, canvas.height);
	nodeRadius = 30;
}



function computeNodeRadius(node, c) {
	if(!seamAutoSize || !node.text) {
		node._r = nodeRadius;
		return nodeRadius;
	}
	c.font = seamFont;
	var lines = node.text.split('\n');
	var maxW = 0;
	for(var i = 0; i < lines.length; i++) {
		var w = c.measureText(convertLatexShortcuts(lines[i])).width;
		if(w > maxW) maxW = w;
	}
	var totalH = lines.length * seamLineHeight;
	// Radius must contain bounding box (maxW × totalH) with padding on all axes
	var r = Math.max(maxW / 2, totalH / 2) + seamAutoSizePad;
	r = Math.max(r, nodeRadius); // never smaller than default
	node._r = r;
	return r;
}

function drawUsing(c) {
	c.clearRect(0, 0, canvas.width, canvas.height);
	c.save();
	c.translate(viewport.x + 0.5, viewport.y + 0.5);
	c.scale(viewport.k, viewport.k);

	_suppressEditBox = true; // edit box drawn in overlay pass after all other elements
	for(var i = 0; i < nodes.length; i++) {
		computeNodeRadius(nodes[i], c);
		c.lineWidth = seamNodeStroke;
		if(nodes[i] == selectedObject) {
			c.fillStyle = c.strokeStyle = 'blue';
			seamActiveLabelColor = 'blue';
		} else {
			c.fillStyle = c.strokeStyle = seamNodeStrokeColor;
			seamActiveLabelColor = seamNodeLabelColor;
		}
		nodes[i].draw(c);
	}
	// Draw halo ring around _haloNode (link-start affordance)
	if(_haloNode != null && _haloOpacity > 0.001) {
		c.save();
		c.beginPath();
		c.arc(_haloNode.x, _haloNode.y, (_haloNode._r != null ? _haloNode._r : nodeRadius) + 14, 0, 2 * Math.PI);
		c.lineWidth = 14;
		c.strokeStyle = 'rgba(99,102,241,' + (0.28 * _haloOpacity) + ')';
		c.stroke();
		c.restore();
	}

	for(var i = 0; i < links.length; i++) {
		// Skip links with stale node references (defensive guard)
		var lnk = links[i];
		if((lnk instanceof Link && (nodes.indexOf(lnk.nodeA) < 0 || nodes.indexOf(lnk.nodeB) < 0)) ||
		   ((lnk instanceof SelfLink || lnk instanceof StartLink) && nodes.indexOf(lnk.node) < 0)) {
			links.splice(i--, 1);
			continue;
		}
		c.lineWidth = seamLinkStroke;
		if(links[i] == selectedObject) {
			c.fillStyle = c.strokeStyle = 'blue';
			seamActiveLabelColor = 'blue';
		} else {
			var _lc = links[i]._id ? (linkColors.get(links[i]._id) || null) : null;
			c.fillStyle = c.strokeStyle = _lc || seamLinkStrokeColor;
			seamActiveLabelColor = seamLinkLabelColor;
		}
		// Halo: semi-transparent wide stroke when hovered
		var isHovered = links[i] === _hoveredLink && links[i] !== selectedObject;
		if(isHovered) {
			c.save();
			c.lineWidth = seamLinkStroke + 10;
			c.strokeStyle = 'rgba(99,102,241,0.14)';
			links[i].drawArcOnly(c);
			c.restore();
		}
		links[i].draw(c);
		// Midpoint dot: animated, appears when mouse is within 10px of line
		if(links[i] !== selectedObject) {
			var showDot = links[i] === _dotLink && _dotOpacity > 0.005;
			if(showDot) {
				var mp = links[i].getMidpoint();
				c.save();
				c.beginPath();
				c.arc(mp.x, mp.y, 3, 0, 2 * Math.PI);
				c.fillStyle = 'rgba(99,102,241,' + (0.75 * _dotOpacity) + ')';
				c.fill();
				c.restore();
			}
		}
	}
	if(currentLink != null) {
		c.lineWidth = seamLinkStroke;
		seamActiveLabelColor = seamLinkLabelColor;
		if(currentLink instanceof TemporaryLink) {
			c.fillStyle = c.strokeStyle = seamLinkStrokeColor;
			c.save(); c.globalAlpha = _linkPulseAlpha;
			currentLink.draw(c);
			// Ghost node when in add-linked-node mode
			if(_addingLinkedNode) {
				var _gr = nodeRadius;
				c.beginPath();
				c.arc(currentLink.to.x, currentLink.to.y, _gr, 0, 2 * Math.PI);
				c.fillStyle = seamNodeFill;
				c.fill();
				c.strokeStyle = seamNodeStrokeColor;
				c.stroke();
			}
			c.restore();
		} else {
			c.fillStyle = c.strokeStyle = seamAccentColor;
			currentLink.draw(c);
		}
	}
	if(_snapBackActive && _snapBackLink != null) {
		c.lineWidth = seamLinkStroke;
		c.fillStyle = c.strokeStyle = seamLinkStrokeColor;
		c.save(); c.globalAlpha = _linkPulseAlpha;
		// Draw snap-back as a TemporaryLink-shaped line+arrow
		c.beginPath();
		c.moveTo(_snapBackLink.to.x, _snapBackLink.to.y);
		c.lineTo(_snapBackLink.from.x, _snapBackLink.from.y);
		c.stroke();
		drawArrow(c, _snapBackLink.to.x, _snapBackLink.to.y,
			Math.atan2(_snapBackLink.to.y - _snapBackLink.from.y, _snapBackLink.to.x - _snapBackLink.from.x));
		c.restore();
	}

	c.restore();

	// Rubber band selection rectangle (drawn in screen space after restore)
	if(_rectSelecting && _rectSelStart && _rectSelCur) {
		var sx1 = (_rectSelStart.x) * viewport.k + viewport.x + 0.5;
		var sy1 = (_rectSelStart.y) * viewport.k + viewport.y + 0.5;
		var sx2 = (_rectSelCur.x)   * viewport.k + viewport.x + 0.5;
		var sy2 = (_rectSelCur.y)   * viewport.k + viewport.y + 0.5;
		c.save();
		c.strokeStyle = 'rgba(99,102,241,0.7)';
		c.lineWidth = 1;
		c.setLineDash([5, 3]);
		c.fillStyle = 'rgba(99,102,241,0.07)';
		c.fillRect(Math.min(sx1,sx2), Math.min(sy1,sy2),
		           Math.abs(sx2-sx1), Math.abs(sy2-sy1));
		c.strokeRect(Math.min(sx1,sx2), Math.min(sy1,sy2),
		             Math.abs(sx2-sx1), Math.abs(sy2-sy1));
		c.setLineDash([]);
		c.restore();
	}

	// Group selection halos — pulsating indigo ring around each selected node
	if(_selectedNodes.length > 0) {
		c.save();
		c.translate(viewport.x + 0.5, viewport.y + 0.5);
		c.scale(viewport.k, viewport.k);
		for(var _ghi = 0; _ghi < _selectedNodes.length; _ghi++) {
			var _ghn = _selectedNodes[_ghi];
			var _ghr = (_ghn._r != null ? _ghn._r : nodeRadius) + 10;
			c.beginPath();
			c.arc(_ghn.x, _ghn.y, _ghr, 0, 2 * Math.PI);
			c.lineWidth = 8;
			c.strokeStyle = 'rgba(99,102,241,' + (0.22 * _groupHaloAlpha) + ')';
			c.stroke();
		}
		c.restore();
	}

	// Edit overlay: draw only the edit box + text for the selected node, above everything
	if(selectedObject instanceof Node && editingLabel && canvasHasFocus() && document.hasFocus()) {
		_suppressEditBox = false;
		c.save();
		c.translate(viewport.x + 0.5, viewport.y + 0.5);
		c.scale(viewport.k, viewport.k);
		c.font = seamFont;
		c.fillStyle = c.strokeStyle = 'blue';
		seamActiveLabelColor = 'blue';
		// Draw only the label text + edit box (not circle, not accept ring)
		drawText(c, selectedObject.text, selectedObject.x, selectedObject.y, null, true);
		// Redraw accept ring after the edit box so it sits above it but ring
		// stays below the caret/text (drawn by drawText already)
		if(selectedObject.isAcceptState) {
			var _ovr = selectedObject._r != null ? selectedObject._r : nodeRadius;
			c.strokeStyle = seamAccentColor;
			c.lineWidth = seamNodeStroke;
			c.beginPath();
			c.arc(selectedObject.x, selectedObject.y, _ovr - 6, 0, 2 * Math.PI, false);
			c.stroke();
		}
		c.restore();
	}
}

function draw() {
	drawUsing(canvas.getContext('2d'));
}

function selectObject(x, y) {
	for(var i = 0; i < nodes.length; i++) {
		if(nodes[i].containsPoint(x, y)) {
			return nodes[i];
		}
	}
	for(var i = 0; i < links.length; i++) {
		if(links[i].containsPoint(x, y)) {
			return links[i];
		}
	}
	return null;
}

function snapNode(node) {
	for(var i = 0; i < nodes.length; i++) {
		if(nodes[i] == node) continue;

		if(Math.abs(node.x - nodes[i].x) < snapToPadding) {
			node.x = nodes[i].x;
		}

		if(Math.abs(node.y - nodes[i].y) < snapToPadding) {
			node.y = nodes[i].y;
		}
	}
}

// Centre and scale all nodes to fit the visible canvas.
// Call after restoreFromBackupData and after canvas dimensions are set.
function zoomToFit() {
	if(!canvas || nodes.length === 0) return;
	var pad = 60;
	var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
	for(var i = 0; i < nodes.length; i++) {
		var _nr = nodes[i]._r != null ? nodes[i]._r : nodeRadius;
		minX = Math.min(minX, nodes[i].x - _nr);
		minY = Math.min(minY, nodes[i].y - _nr);
		maxX = Math.max(maxX, nodes[i].x + _nr);
		maxY = Math.max(maxY, nodes[i].y + _nr);
	}
	var graphW = maxX - minX + pad * 2;
	var graphH = maxY - minY + pad * 2;
	var k = Math.min(canvas.width / graphW, canvas.height / graphH, 1.5);
	viewport.k = k;
	viewport.x = canvas.width  / 2 - (minX + (maxX - minX) / 2) * k;
	viewport.y = canvas.height / 2 - (minY + (maxY - minY) / 2) * k;
	draw();
}

// Zoom by a scale factor toward the canvas centre.
// factor > 1 zooms in; factor < 1 zooms out.
function zoomBy(factor) {
	if (!canvas) return;
	var newK = Math.max(0.15, Math.min(5, viewport.k * factor));
	factor = newK / viewport.k;
	var cx = canvas.width  / 2;
	var cy = canvas.height / 2;
	viewport.k  = newK;
	viewport.x  = cx - (cx - viewport.x) * factor;
	viewport.y  = cy - (cy - viewport.y) * factor;
	draw();
}


// ── showDialog ────────────────────────────────────────────────────────────────
// Portable Promise-based modal.
function showFSMDialog(opts) {
	return new Promise(function(resolve) {
		var dark = document.documentElement.classList.contains('dark');
		var backdrop = document.createElement('div');
		Object.assign(backdrop.style, {
			position:'fixed', inset:'0',
			background:'rgba(0,0,0,0)', backdropFilter:'blur(2px)',
			zIndex:'9999', display:'flex', alignItems:'center', justifyContent:'center',
			transition:'background 220ms ease',
		});
		var modal = document.createElement('div');
		Object.assign(modal.style, {
			background: dark ? '#1e293b' : '#ffffff',
			borderRadius:'12px', boxShadow:'0 20px 60px rgba(0,0,0,0.3)',
			width:'400px', maxWidth:'calc(100vw - 32px)',
			overflow:'hidden', fontFamily:'inherit',
			opacity:'0', transform:'scale(0.70)',
			transition:'opacity 200ms ease, transform 200ms ease',
		});
		// Header
		var header = document.createElement('div');
		Object.assign(header.style, {
			padding:'16px 20px',
			borderBottom:'1px solid ' + (dark ? '#334155' : '#e2e8f0'),
			display:'flex', alignItems:'center', justifyContent:'space-between',
		});
		var titleEl = document.createElement('div');
		Object.assign(titleEl.style, { fontWeight:'600', fontSize:'15px', color: dark ? '#f1f5f9' : '#0f172a' });
		titleEl.textContent = opts.title;
		var closeBtn = document.createElement('button');
		closeBtn.innerHTML = '<span class="material-icons" style="font-size:18px;line-height:1">close</span>';
		Object.assign(closeBtn.style, {
			background:'none', border:'none', cursor:'pointer',
			color: dark ? '#94a3b8' : '#64748b', padding:'2px', borderRadius:'4px',
			display:'flex', alignItems:'center',
		});
		function _dismiss(result) {
			document.removeEventListener('keydown', onKey);
			backdrop.style.background = 'rgba(0,0,0,0)';
			modal.style.opacity = '0';
			modal.style.transform = 'scale(0.70)';
			setTimeout(function() {
				if(backdrop.parentNode) backdrop.parentNode.removeChild(backdrop);
				resolve(result);
			}, 220);
		}
		closeBtn.onclick = function() { _dismiss(null); };
		header.append(titleEl, closeBtn);
		// Body
		var body = document.createElement('div');
		body.style.cssText = 'padding:16px 20px;display:flex;flex-direction:column;gap:14px;';
		var inputs = {};
		for(var fi = 0; fi < opts.fields.length; fi++) {
			var field = opts.fields[fi];
			var wrap = document.createElement('div');
			var lbl = document.createElement('label');
			lbl.textContent = field.label;
			Object.assign(lbl.style, {
				display:'block', fontSize:'11px', fontWeight:'500',
				color: dark ? '#94a3b8' : '#64748b',
				marginBottom:'4px', textTransform:'uppercase', letterSpacing:'0.05em',
			});
			var inp = document.createElement('input');
			inp.type = 'text';
			inp.value = field.value || '';
			inp.placeholder = field.placeholder || '';
			Object.assign(inp.style, {
				width:'100%', boxSizing:'border-box',
				padding:'7px 10px', fontSize:'13px', borderRadius:'6px',
				border:'1px solid ' + (dark ? '#475569' : '#cbd5e1'),
				background: dark ? '#0f172a' : '#f8fafc',
				color: dark ? '#e2e8f0' : '#1e293b', outline:'none',
			});
			(function(i) {
				i.addEventListener('focus', function() { i.style.borderColor='#6366f1'; i.style.boxShadow='0 0 0 3px rgba(99,102,241,0.15)'; });
				i.addEventListener('blur',  function() { i.style.borderColor = dark ? '#475569' : '#cbd5e1'; i.style.boxShadow='none'; });
				i.addEventListener('keydown', function(ev) { if(ev.key === 'Enter') { saveBtn.click(); ev.preventDefault(); } });
			})(inp);
			inputs[field.key] = inp;
			wrap.append(lbl, inp);
			body.appendChild(wrap);
		}
		// Footer
		var footer = document.createElement('div');
		Object.assign(footer.style, {
			padding:'12px 20px', borderTop:'1px solid ' + (dark ? '#334155' : '#e2e8f0'),
			display:'flex', justifyContent:'flex-end', gap:'8px',
		});
		var cancelBtn = document.createElement('button');
		cancelBtn.textContent = 'Cancel';
		Object.assign(cancelBtn.style, {
			padding:'6px 14px', fontSize:'13px', borderRadius:'6px',
			border:'1px solid ' + (dark ? '#475569' : '#cbd5e1'),
			background:'none', cursor:'pointer', color: dark ? '#94a3b8' : '#64748b',
		});
		cancelBtn.onclick = function() { _dismiss(null); };
		var saveBtn = document.createElement('button');
		saveBtn.textContent = 'Save';
		Object.assign(saveBtn.style, {
			padding:'6px 16px', fontSize:'13px', fontWeight:'600',
			borderRadius:'6px', border:'none', cursor:'pointer',
			background:'#6366f1', color:'#ffffff',
		});
		saveBtn.addEventListener('mouseover', function() { saveBtn.style.background='#4f46e5'; });
		saveBtn.addEventListener('mouseout',  function() { saveBtn.style.background='#6366f1'; });
		saveBtn.onclick = function() {
			var result = {};
			for(var k in inputs) result[k] = inputs[k].value;
			_dismiss(result);
		};
		footer.append(cancelBtn, saveBtn);
		modal.append(header, body, footer);
		backdrop.appendChild(modal);
		document.body.appendChild(backdrop);
		// Double rAF: first frame commits initial styles,
		// second frame triggers the transition from them.
		requestAnimationFrame(function() {
			requestAnimationFrame(function() {
				backdrop.style.background = 'rgba(0,0,0,0.45)';
				modal.style.opacity = '1';
				modal.style.transform = 'scale(1)';
			});
		});
		setTimeout(function() { var first = inputs[opts.fields[0].key]; if(first) first.focus(); }, 50);
		var onKey = function(ev) {
			if(ev.key === 'Escape') { _dismiss(null); }
		};
		document.addEventListener('keydown', onKey);
	});
}

// Ensure each link gets a stable _id for the properties map
function ensureLinkId(link) {
	if(!link._id) link._id = ++_linkIdCounter;
	return link._id;
}

function initFSM(canvasEl) {
	canvas = canvasEl;

	canvas.oncontextmenu = function(e) { e.preventDefault(); return false; };

	canvas.onmousedown = function(e) {
		var mouse = crossBrowserRelativeMousePos(e);
		movingObject = false;
		originalClick = mouse;
		editingLabel = false;
		_hasDragged  = false;

		// Halo click: left=connect-link, right=add-linked-node
		if(_haloNode != null && _haloVisible && !e.shiftKey) {
			var hn = _haloNode;
			_haloNode    = null;
			_haloVisible = false;
			_haloOpacity = 0;
			_haloTarget  = 0;
			selectedObject = hn;
			if(e.button === 2) {
				// Right-click halo: add-linked-node mode
				_addingLinkedNode = true;
				_addLinkedOrigin  = hn;
			} else {
				_addingLinkedNode = false;
				_addLinkedOrigin  = null;
			}
			currentLink = new TemporaryLink(hn.closestPointOnCircle(mouse.x, mouse.y), mouse);
			_startLinkPulse();
			canvas.style.cursor = 'crosshair';
			draw();
			return false;
		}

		// Clear group selection on any left-click (unless we hit a selected node)
		var _hitSelected = false;
		if(_selectedNodes.length > 0) {
			var _hm = selectObject(mouse.x, mouse.y);
			if(_hm instanceof Node && _selectedNodes.indexOf(_hm) >= 0) {
				_hitSelected = true;
				// Move the whole group (any mouse button)
				movingObject = true;
				_hasDragged = false;
				for(var _gi = 0; _gi < _selectedNodes.length; _gi++) {
					_selectedNodes[_gi].setMouseStart(mouse.x, mouse.y);
				}
				canvas.style.cursor = 'grabbing';
				draw();
				return false;
			}
			// Clicked away from any selected node — clear on mouseup if no drag
		}
		selectedObject = selectObject(mouse.x, mouse.y);
		if(selectedObject != null) {
			caretPosition = (selectedObject.text || '').length;
			if(e.shiftKey && selectedObject instanceof Node) {
				// Shift+click on node toggles accept (green) state
				selectedObject.isAcceptState = !selectedObject.isAcceptState;
				selectedObject = null; // deselect so no label edit follows
				draw();
				return false;
			} else {
				movingObject = true;
				deltaMouseX = deltaMouseY = 0;
				if(selectedObject.setMouseStart) {
					selectedObject.setMouseStart(mouse.x, mouse.y);
				}
				canvas.style.cursor = 'grabbing';
			}
			// resetCaret called only on stationary mouseup
		} else if(e.button === 2) {
			// Right-drag on empty canvas: start rubber-band group selection
			_rectSelecting = true;
			_rectSelStart  = { x: mouse.x, y: mouse.y };
			_rectSelCur    = { x: mouse.x, y: mouse.y };
			_selectedNodes = [];
			canvas.style.cursor = 'crosshair';
		} else {
			// Left-drag on empty canvas: pan
			panningCanvas = true;
			panStart = crossBrowserRawMousePos(e);
			panViewStart = { x: viewport.x, y: viewport.y };
			canvas.style.cursor = 'grabbing';
		}

		draw();

		if(canvasHasFocus()) {
			// disable drag-and-drop only if the canvas is already focused
			return false;
		} else {
			// otherwise, let the browser switch the focus away from wherever it was
			resetCaret();
			return true;
		}
	};

	canvas.ondblclick = function(e) {
		var mouse = crossBrowserRelativeMousePos(e);
		selectedObject = selectObject(mouse.x, mouse.y);

		if(selectedObject == null) {
			selectedObject = new Node(mouse.x, mouse.y);
			nodes.push(selectedObject);
			resetCaret();
			_startNodeGrow(selectedObject);
			draw();
		}
	};

	canvas.onmousemove = function(e) {
		var mouse = crossBrowserRelativeMousePos(e);

		// Skip link tracking during snap-back animation
		if(_snapBackActive) return;

		if(currentLink != null) {
			var targetNode = selectObject(mouse.x, mouse.y);
			if(!(targetNode instanceof Node)) {
				targetNode = null;
			}

			if(selectedObject == null) {
				if(targetNode != null) {
					_stopLinkPulse();
					currentLink = new StartLink(targetNode, originalClick);
				} else {
					currentLink = new TemporaryLink(originalClick, mouse);
					_startLinkPulse();
				}
			} else {
				if(targetNode == selectedObject) {
					_stopLinkPulse();
					currentLink = new SelfLink(selectedObject, mouse);
				} else if(targetNode != null) {
					_stopLinkPulse();
					currentLink = new Link(selectedObject, targetNode);
				} else {
					currentLink = new TemporaryLink(selectedObject.closestPointOnCircle(mouse.x, mouse.y), mouse);
					_startLinkPulse();
				}
			}
			draw();
		}

		if(movingObject) {
			_hasDragged = true;
			if(_selectedNodes.length > 0 && selectedObject == null) {
				// Group drag
				for(var _gmi = 0; _gmi < _selectedNodes.length; _gmi++) {
					_selectedNodes[_gmi].setAnchorPoint(mouse.x, mouse.y);
				}
			} else if(selectedObject != null) {
				selectedObject.setAnchorPoint(mouse.x, mouse.y);
				if(selectedObject instanceof Node) {
					snapNode(selectedObject);
				}
			}
			draw();
		}

		if(panningCanvas) {
			var raw = crossBrowserRawMousePos(e);
			viewport.x = panViewStart.x + (raw.x - panStart.x);
			viewport.y = panViewStart.y + (raw.y - panStart.y);
			draw();
		}
		if(_rectSelecting) {
			_hasDragged = true;
			_rectSelCur = { x: mouse.x, y: mouse.y };
			draw();
		}

		// Hover cursor and halo state when nothing is being dragged
		if(!movingObject && !panningCanvas && currentLink == null) {
			var onNode = null;
			for(var _ni = 0; _ni < nodes.length; _ni++) {
				if(nodes[_ni].containsPoint(mouse.x, mouse.y)) { onNode = nodes[_ni]; break; }
			}
			if(onNode != null) {
				// Inside a node: prime _haloNode, start fade-out if visible
				_haloNode    = onNode;
				_haloVisible = false;
				if(_haloTarget !== 0) { _haloTarget = 0; _startHaloAnim(); }
				if(_hoveredLink !== null) { _hoveredLink = null; draw(); }
				if(_dotTarget !== 0) { _dotTarget = 0; _startDotAnim(); }
				canvas.style.cursor = 'grab';
			} else if(_haloNode != null) {
				// Outside all nodes: check halo zone distance
				var _dx = mouse.x - _haloNode.x, _dy = mouse.y - _haloNode.y;
				var _hr = _haloNode._r != null ? _haloNode._r : nodeRadius;
				var _dist = Math.sqrt(_dx*_dx + _dy*_dy) - _hr;
				if(_dist > 0 && _dist <= 20) {
					_haloVisible = true;
					if(_haloTarget !== 1) { _haloTarget = 1; _startHaloAnim(); }
					canvas.style.cursor = 'cell';
					if(_hoveredLink !== null) { _hoveredLink = null; draw(); }
				} else {
					_haloVisible = false;
					if(_haloTarget !== 0) { _haloTarget = 0; _startHaloAnim(); }
					var _hov = selectObject(mouse.x, mouse.y);
					canvas.style.cursor = (_hov != null) ? 'grab' : 'crosshair';
				}
			} else {
				var _hov2 = selectObject(mouse.x, mouse.y);
				canvas.style.cursor = (_hov2 != null) ? 'grab' : 'crosshair';
			}
			// _hoveredLink and dot: unconditional within idle block
			var _hlCheck = selectObject(mouse.x, mouse.y);
			var _nl = (_hlCheck instanceof Link || _hlCheck instanceof SelfLink || _hlCheck instanceof StartLink) ? _hlCheck : null;
			if(_nl !== _hoveredLink) { _hoveredLink = _nl; draw(); }
			// Dot proximity check: unconditional, 30px tolerance
			var _oldPad = hitTargetPadding;
			hitTargetPadding = 30;
			var _near = selectObject(mouse.x, mouse.y);
			hitTargetPadding = _oldPad;
			var _dotCandidate = (_near instanceof Link || _near instanceof SelfLink || _near instanceof StartLink) ? _near : null;
			if(_dotCandidate !== _dotLink) {
				if(_dotCandidate != null) {
					_dotLink = _dotCandidate;
					_dotTarget = 1;
				} else {
					_dotTarget = 0;
				}
				_startDotAnim();
			}
		}
	};

	canvas.onmouseup = function(e) {
		var didDrag = _hasDragged;
		movingObject = false;
		panningCanvas = false;
		if(_rectSelecting) {
			_rectSelecting = false;
			// Collect nodes inside rect
			if(_rectSelStart && _rectSelCur) {
				var rx1 = Math.min(_rectSelStart.x, _rectSelCur.x);
				var ry1 = Math.min(_rectSelStart.y, _rectSelCur.y);
				var rx2 = Math.max(_rectSelStart.x, _rectSelCur.x);
				var ry2 = Math.max(_rectSelStart.y, _rectSelCur.y);
				_selectedNodes = [];
				for(var _ri = 0; _ri < nodes.length; _ri++) {
					var _rn = nodes[_ri];
					if(_rn.x >= rx1 && _rn.x <= rx2 && _rn.y >= ry1 && _rn.y <= ry2) {
						_selectedNodes.push(_rn);
					}
				}
				if(_selectedNodes.length > 0) _startGroupHalo();
			}
			_rectSelStart = _rectSelCur = null;
			canvas.style.cursor = 'crosshair';
			draw();
			return;
		}
		_hasDragged = false;

		// Any stationary click clears group selection
		if(!didDrag && _selectedNodes.length > 0) {
			_selectedNodes = [];
			_groupHaloRafId = null;
			draw();
		}

		// Shift+click on a committed link: cycle its colour
		if(!didDrag && e.shiftKey && currentLink == null &&
		   selectedObject != null && !(selectedObject instanceof Node)) {
			ensureLinkId(selectedObject);
			var _cycle = [seamAccentColor, '#4b0082', '#8b0000', null];
			var _cur = linkColors.get(selectedObject._id) || null;
			var _idx = _cycle.indexOf(_cur);
			var _next = _cycle[(_idx + 1) % _cycle.length];
			if(_next === null) linkColors.delete(selectedObject._id);
			else linkColors.set(selectedObject._id, _next);
			selectedObject = null; // deselect so custom colour is immediately visible
			draw();
		// Activate label editing only on a stationary click with no link being committed
		} else if(!didDrag && !e.shiftKey && currentLink == null &&
		          selectedObject != null && 'text' in selectedObject) {
			editingLabel = true;
			resetCaret();
		}

		// Always restore cursor based on what is under the mouse
		var hovered = selectObject(crossBrowserRelativeMousePos(e).x, crossBrowserRelativeMousePos(e).y);
		canvas.style.cursor = (hovered != null) ? 'grab' : 'crosshair';

			if(_addingLinkedNode && currentLink instanceof TemporaryLink) {
				_addingLinkedNode = false;
				var _dropTarget = selectObject(mouse.x, mouse.y);
				if(_dropTarget instanceof Node && _dropTarget !== _addLinkedOrigin) {
					// Dropped on existing node — commit as normal link
					_stopLinkPulse();
					var _newLink = new Link(_addLinkedOrigin, _dropTarget);
					links.push(_newLink);
					selectedObject = _newLink;
					resetCaret();
				} else {
					// Dropped on empty canvas — create new node + link
					_stopLinkPulse();
					var _newNode = new Node(mouse.x, mouse.y);
					nodes.push(_newNode);
					var _newLink2 = new Link(_addLinkedOrigin, _newNode);
					links.push(_newLink2);
					selectedObject = _newNode;
					resetCaret();
					_startNodeGrow(_newNode);
				}
				_addLinkedOrigin = null;
				currentLink = null;
				draw();
			} else if(currentLink != null) {
				if(currentLink instanceof TemporaryLink) {
					// Released without connecting: snap back to origin
					_startSnapBack(currentLink);
				} else {
					_stopLinkPulse();
					// Validate all node references before committing
					var valid = true;
					if(currentLink instanceof Link) {
						valid = nodes.indexOf(currentLink.nodeA) >= 0 && nodes.indexOf(currentLink.nodeB) >= 0;
					} else if(currentLink instanceof SelfLink) {
						valid = nodes.indexOf(currentLink.node) >= 0;
					} else if(currentLink instanceof StartLink) {
						valid = nodes.indexOf(currentLink.node) >= 0;
					}
					if(valid) {
						selectedObject = currentLink;
						links.push(currentLink);
						resetCaret();
					}
					currentLink = null;
					draw();
				}
			}
	};

	// Cancel drag/pan/link if mouse leaves canvas
	canvas.onmouseleave = function() {
		_haloVisible = false;
		if(_haloTarget !== 0) { _haloTarget = 0; _startHaloAnim(); }
		var _leaveRedraw = _hoveredLink !== null;
		_hoveredLink = null;
		if(_dotTarget !== 0) { _dotTarget = 0; _startDotAnim(); }
		if(currentLink instanceof TemporaryLink) {
			_startSnapBack(currentLink);
			movingObject = false;
			panningCanvas = false;
			canvas.style.cursor = 'crosshair';
		} else if(movingObject || panningCanvas) {
			movingObject = false;
			panningCanvas = false;
			currentLink = null;
			canvas.style.cursor = 'crosshair';
			draw();
		} else if(_leaveRedraw) {
			draw();
		}
	};

	// Right-click on a link: open transition properties dialog
	canvas.oncontextmenu = function(e) {
		e.preventDefault();
		var mouse = crossBrowserRelativeMousePos(e);
		var target = selectObject(mouse.x, mouse.y);
		if(target == null || target instanceof Node) return;
		ensureLinkId(target);
		var props = linkProperties.get(target._id) || { guard: '', action: '', output: '' };
		showFSMDialog({
			title: 'Transition Properties',
			fields: [
				{ key: 'label',  label: 'Label',  value: target.text,   placeholder: 'e.g. unlock' },
				{ key: 'guard',  label: 'Guard',  value: props.guard,  placeholder: 'e.g. key_valid == true' },
				{ key: 'action', label: 'Action', value: props.action, placeholder: 'e.g. notify:email' },
				{ key: 'output', label: 'Output', value: props.output, placeholder: 'e.g. door_opened' },
			],
		}).then(function(result) {
			if(result == null) return;
			target.text = result.label;
			linkProperties.set(target._id, { guard: result.guard, action: result.action, output: result.output });
			draw();
		});
	};

	// Wheel zoom with inertia — accumulates velocity in log-space,
	// then decays via requestAnimationFrame for a smooth continuous feel.
	canvas.onwheel = function(e) {
		e.preventDefault();
		var raw = crossBrowserRawMousePos(e);
		// Cursor position: update on each event so fast flicks track correctly
		_zoomCursorX = raw.x;
		_zoomCursorY = raw.y;
		// Accumulate in log-space: uniform feel at any zoom level
		var sensitivity = 0.0008;
		_zoomVelocity += e.deltaY * sensitivity;
		if(_zoomRafId === null) {
			var lastTime = performance.now();
			function zoomFrame(now) {
				var dt = Math.min(now - lastTime, 64); // cap dt to avoid jumps after tab switch
				lastTime = now;
				var step = _zoomVelocity * dt * 0.05;
				if(Math.abs(step) > 0.0001) {
					var factor = Math.exp(-step);
					var newK = Math.max(0.15, Math.min(5, viewport.k * factor));
					factor = newK / viewport.k;
					viewport.k = newK;
					viewport.x = _zoomCursorX - (_zoomCursorX - viewport.x) * factor;
					viewport.y = _zoomCursorY - (_zoomCursorY - viewport.y) * factor;
					draw();
					_zoomVelocity *= 0.801; // decay (tuned for ~10% longer inertia than 0.78)
					_zoomRafId = requestAnimationFrame(zoomFrame);
				} else {
					_zoomVelocity = 0;
					_zoomRafId = null;
				}
			}
			_zoomRafId = requestAnimationFrame(zoomFrame);
		}
		return false;
	};
}

document.addEventListener('keydown', function(e) {
	// Only active when the Wallace FSM canvas is in the page
	if (!canvas) return;
	var key = crossBrowserKey(e);

	if(!canvasHasFocus()) {
		// don't read keystrokes when other things have focus
		return true;
	} else if(key == 8) { // Backspace — delete char before caret (or delete selected when not editing)
		if(selectedObject != null && 'text' in selectedObject && editingLabel && caretPosition > 0) {
			var t = selectedObject.text;
			selectedObject.text = t.substring(0, caretPosition - 1) + t.substring(caretPosition);
			caretPosition = Math.max(0, caretPosition - 1);
			resetCaret();
			draw();
		} else if(selectedObject != null && !editingLabel) {
			// Not editing: backspace deletes the selected node/link (same as Delete)
			for(var _bi = 0; _bi < nodes.length; _bi++) {
				if(nodes[_bi] == selectedObject) { nodes.splice(_bi--, 1); }
			}
			for(var _bi = 0; _bi < links.length; _bi++) {
				if(links[_bi] == selectedObject || links[_bi].node == selectedObject ||
				   links[_bi].nodeA == selectedObject || links[_bi].nodeB == selectedObject) {
					links.splice(_bi--, 1);
				}
			}
			selectedObject = null;
			editingLabel = false;
			draw();
		}
		// backspace is a shortcut for the back button, but do NOT want to change pages
		return false;
	} else if(key == 13) { // Enter
		if(e.altKey && selectedObject != null && 'text' in selectedObject && editingLabel) {
			// Alt+Enter: insert newline
			var t = selectedObject.text || '';
			selectedObject.text = t.substring(0, caretPosition) + '\n' + t.substring(caretPosition);
			caretPosition++;
			resetCaret(); draw();
			return false;
		} else {
			selectedObject = null; editingLabel = false; caretPosition = 0; draw();
		}
	} else if(key == 27) { // Escape — commit label
		selectedObject = null; editingLabel = false; caretPosition = 0; draw();
	} else if(key == 37) { // Arrow Left
		if(selectedObject != null && 'text' in selectedObject && editingLabel) {
			caretPosition = Math.max(0, caretPosition - 1);
			resetCaret(); draw();
			return false;
		}
	} else if(key == 39) { // Arrow Right
		if(selectedObject != null && 'text' in selectedObject && editingLabel) {
			caretPosition = Math.min((selectedObject.text || '').length, caretPosition + 1);
			resetCaret(); draw();
			return false;
		}
	} else if(key == 36) { // Home
		if(selectedObject != null && 'text' in selectedObject && editingLabel) {
			caretPosition = 0;
			resetCaret(); draw();
			return false;
		}
	} else if(key == 35) { // End
		if(selectedObject != null && 'text' in selectedObject && editingLabel) {
			caretPosition = (selectedObject.text || '').length;
			resetCaret(); draw();
			return false;
		}
	} else if(key == 38) { // Arrow Up — move caret up one line
		if(selectedObject != null && 'text' in selectedObject && editingLabel) {
			var t = selectedObject.text || '';
			var lc = caretToLineCol(t, caretPosition);
			if(lc.line > 0) caretPosition = lineColToPos(t, lc.line - 1, lc.col);
			resetCaret(); draw(); return false;
		}
	} else if(key == 40) { // Arrow Down — move caret down one line
		if(selectedObject != null && 'text' in selectedObject && editingLabel) {
			var t = selectedObject.text || '';
			var lc = caretToLineCol(t, caretPosition);
			if(lc.line < t.split('\n').length - 1) caretPosition = lineColToPos(t, lc.line + 1, lc.col);
			resetCaret(); draw(); return false;
		}
	} else if(key == 46) { // Delete — remove selected node/link
		if(selectedObject != null) {
			for(var i = 0; i < nodes.length; i++) {
				if(nodes[i] == selectedObject) {
					nodes.splice(i--, 1);
				}
			}
			for(var i = 0; i < links.length; i++) {
				if(links[i] == selectedObject || links[i].node == selectedObject || links[i].nodeA == selectedObject || links[i].nodeB == selectedObject) {
					links.splice(i--, 1);
				}
			}
			selectedObject = null;
			editingLabel = false;
			draw();
		}
	}
});

document.addEventListener('keyup', function(e) {
	if (!canvas) return;
	var key = crossBrowserKey(e);

});

document.addEventListener('keypress', function(e) {
	if (!canvas) return;
	// don't read keystrokes when other things have focus
	var key = crossBrowserKey(e);
	if(!canvasHasFocus()) {
		// don't read keystrokes when other things have focus
		return true;
	} else if(key >= 0x20 && key <= 0x7E && !e.metaKey && !e.altKey && !e.ctrlKey && selectedObject != null && 'text' in selectedObject && editingLabel) {
		var ch = String.fromCharCode(key);
		var t = selectedObject.text || '';
		selectedObject.text = t.substring(0, caretPosition) + ch + t.substring(caretPosition);
		caretPosition++;
		resetCaret();
		draw();

		// don't let keys do their actions (like space scrolls down the page)
		return false;
	} else if(key == 8) {
		// backspace is a shortcut for the back button, but do NOT want to change pages
		return false;
	}
});

function crossBrowserKey(e) {
	e = e || window.event;
	return e.which || e.keyCode;
}

// Raw canvas-space position using e.offsetX/Y (reliable in embedded context).
// Wallace's original offsetParent traversal fails inside complex layouts.
function crossBrowserRawMousePos(e) {
	return { 'x': e.offsetX, 'y': e.offsetY };
}

// World-space position (corrected for viewport pan and zoom).
function crossBrowserRelativeMousePos(e) {
	var raw = crossBrowserRawMousePos(e);
	return {
		'x': (raw.x - viewport.x) / viewport.k,
		'y': (raw.y - viewport.y) / viewport.k
	};
}

function output(text) {
	var element = document.getElementById('output');
	if (!element) return;
	element.style.display = 'block';
	element.value = text;
}





// Compute world-space bounding box from node geometry — reliable regardless
// of canvas size or current zoom/pan. Returns [minX, minY, maxX, maxY].
function getGeometryBounds() {
	if(!nodes.length) return [0, 0, canvas.width || 600, canvas.height || 400];
	var pad = 60;
	var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
	for(var i = 0; i < nodes.length; i++) {
		var r = nodes[i]._r != null ? nodes[i]._r : nodeRadius;
		minX = Math.min(minX, nodes[i].x - r);
		minY = Math.min(minY, nodes[i].y - r);
		maxX = Math.max(maxX, nodes[i].x + r);
		maxY = Math.max(maxY, nodes[i].y + r);
	}
	// Expand for link labels and arrowheads
	minX -= pad; minY -= pad; maxX += pad; maxY += pad;
	return [minX, minY, maxX, maxY];
}

// Returns the FSM as an SVG string at world scale (viewport reset to identity).\n// Clears transient visual state (halo, dot) during export so they do not appear.
function getSVGString() {
	var oldSelected = selectedObject;
	var savedViewport = { x: viewport.x, y: viewport.y, k: viewport.k };
	var savedHalo = _haloOpacity, savedDot = _dotOpacity;
	selectedObject = null;
	viewport = { x: 0, y: 0, k: 1 };
	_haloOpacity = 0; _dotOpacity = 0;
	// Use geometry-based bounds — not pixel scan — so the full graph is captured
	// regardless of canvas size or how far nodes extend beyond the visible area.
	var bounds = getGeometryBounds();
	// Render to SVG exporter
	var exporter = new ExportAsSVG(bounds);
	drawUsing(exporter);
	var svg = exporter.toSVG();
	// Restore state
	selectedObject = oldSelected;
	viewport = savedViewport;
	_haloOpacity = savedHalo; _dotOpacity = savedDot;
	draw(); // restore visual
	return svg;
}


function saveAsLaTeX(monochrome) {
	var bounds = getGeometryBounds();
	var exporter = new ExportAsLaTeX(bounds, monochrome);
	var oldSelectedObject = selectedObject;
	selectedObject = null;
	_haloOpacity = 0; _dotOpacity = 0;
	drawUsing(exporter);
	selectedObject = oldSelectedObject;
	draw();
	var texData = exporter.toLaTeX();
	var blob = new Blob([texData], { type: 'application/x-latex' });
	var url = URL.createObjectURL(blob);
	var a = document.createElement('a');
	a.href = url;
	a.download = monochrome ? 'fsm-diagram-mono.tex' : 'fsm-diagram.tex';
	document.body.appendChild(a); a.click(); document.body.removeChild(a);
	URL.revokeObjectURL(url);
}




function det(a, b, c, d, e, f, g, h, i) {
	return a*e*i + b*f*g + c*d*h - a*f*h - b*d*i - c*e*g;
}

function circleFromThreePoints(x1, y1, x2, y2, x3, y3) {
	var a = det(x1, y1, 1, x2, y2, 1, x3, y3, 1);
	var bx = -det(x1*x1 + y1*y1, y1, 1, x2*x2 + y2*y2, y2, 1, x3*x3 + y3*y3, y3, 1);
	var by = det(x1*x1 + y1*y1, x1, 1, x2*x2 + y2*y2, x2, 1, x3*x3 + y3*y3, x3, 1);
	var c = -det(x1*x1 + y1*y1, x1, y1, x2*x2 + y2*y2, x2, y2, x3*x3 + y3*y3, x3, y3);
	return {
		'x': -bx / (2*a),
		'y': -by / (2*a),
		'radius': Math.sqrt(bx*bx + by*by - 4*a*c) / (2*Math.abs(a))
	};
}

function fixed(number, digits) {
	return number.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '');
}

function restoreFromBackupData(backup) {
	for(var i = 0; i < backup.nodes.length; i++) {
		var backupNode = backup.nodes[i];
		var node = new Node(backupNode.x, backupNode.y);
		node.isAcceptState = backupNode.isAcceptState;
		node.text = backupNode.text;
		node.textOnly = backupNode.textOnly;
		nodes.push(node);
	}
	for(var i = 0; i < backup.links.length; i++) {
		var backupLink = backup.links[i];
		var link = null;
		if(backupLink.type == 'SelfLink') {
			link = new SelfLink(nodes[backupLink.node]);
			link.anchorAngle = backupLink.anchorAngle;
			link.text = backupLink.text;
		} else if(backupLink.type == 'StartLink') {
			link = new StartLink(nodes[backupLink.node]);
			link.deltaX = backupLink.deltaX;
			link.deltaY = backupLink.deltaY;
			link.text = backupLink.text;
		} else if(backupLink.type == 'Link') {
			link = new Link(nodes[backupLink.nodeA], nodes[backupLink.nodeB]);
			link.parallelPart = backupLink.parallelPart;
			link.perpendicularPart = backupLink.perpendicularPart;
			link.text = backupLink.text;
			link.lineAngleAdjust = backupLink.lineAngleAdjust;
		}
		if(link != null) {
			links.push(link);
			if(backupLink.guard || backupLink.action || backupLink.output || backupLink.color) {
				ensureLinkId(link);
				linkProperties.set(link._id, { guard: backupLink.guard || '', action: backupLink.action || '', output: backupLink.output || '' });
				if(backupLink.color) linkColors.set(link._id, backupLink.color);
			}
		}
	}
	nodeRadius = backup.nodeRadius;
	draw();
}


function getBackupData() {
	var backup = {
		'nodes': [],
		'links': [],
		'nodeRadius': nodeRadius,
	};
	for(var i = 0; i < nodes.length; i++) {
		var node = nodes[i];
		var backupNode = {
			'x': node.x,
			'y': node.y,
			'text': node.text,
			'isAcceptState': node.isAcceptState,
			'textOnly': node.textOnly,
		};
		backup.nodes.push(backupNode);
	}
	for(var i = 0; i < links.length; i++) {
		var link = links[i];
		var backupLink = null;
		var lp = link._id ? (linkProperties.get(link._id) || {}) : {};
		var lc = link._id ? (linkColors.get(link._id) || null) : null;
		if(link instanceof SelfLink) {
		backupLink = {
				'type': 'SelfLink',
				'node': nodes.indexOf(link.node),
				'text': link.text,
				'anchorAngle': link.anchorAngle,
				'guard': lp.guard || '',
				'action': lp.action || '',
				'output': lp.output || '',
				'color': lc || '',
			};
		} else if(link instanceof StartLink) {
			backupLink = {
				'type': 'StartLink',
				'node': nodes.indexOf(link.node),
				'text': link.text,
				'deltaX': link.deltaX,
				'deltaY': link.deltaY,
				'guard': lp.guard || '',
				'action': lp.action || '',
				'output': lp.output || '',
				'color': lc || '',
			};
		} else if(link instanceof Link) {
			backupLink = {
				'type': 'Link',
				'nodeA': nodes.indexOf(link.nodeA),
				'nodeB': nodes.indexOf(link.nodeB),
				'text': link.text,
				'lineAngleAdjust': link.lineAngleAdjust,
				'parallelPart': link.parallelPart,
				'perpendicularPart': link.perpendicularPart,
				'guard': lp.guard || '',
				'action': lp.action || '',
				'output': lp.output || '',
				'color': lc || '',
			};
		}
		if(backupLink != null) {
			backup.links.push(backupLink);
		}
	}
	return backup;
}

