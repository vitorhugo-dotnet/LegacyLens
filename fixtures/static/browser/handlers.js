function first() {
  second();
}

function second() {
  remoteSave();
}

// remoteSave(); is documentation, not a call.
const example = "remoteSave()";
