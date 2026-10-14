import assert from "node:assert/strict";
import { test } from "node:test";
import { createRequire } from "node:module";
import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  symlinkSync,
  rmSync,
} from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

const requireWeb = createRequire(resolve("apps/web/package.json"));
const { parse, compileScript } = requireWeb("@vue/compiler-sfc");
const { ref, computed, watch, reactive, nextTick, effectScope } =
  requireWeb("vue");
const flush = async () => {
  await nextTick();
  await new Promise((resolve) => setImmediate(resolve));
  await nextTick();
};
const box = { x: 0.2, y: 0.2, width: 0.4, height: 0.5 };
const ready = {
  faces: [{ box }],
  face_count: 1,
  quality: "ready",
  message: "Wajah siap",
};

async function setup(request, enabled = true, deferEmit = false) {
  let unmount;
  Object.assign(globalThis, {
    ref,
    computed,
    watch,
    nextTick,
    onBeforeUnmount: (fn) => {
      unmount = fn;
    },
    useApi: () => request,
    apiMessage: () => "Layanan belum tersedia.",
  });
  const directory = resolve(".tools/enrollment-regression");
  mkdirSync(directory, { recursive: true });
  try {
    symlinkSync(
      resolve("apps/web/node_modules"),
      directory + "/node_modules",
      "dir",
    );
  } catch (error) {
    if (error.code !== "EEXIST") throw error;
  }
  const { descriptor } = parse(
    readFileSync("apps/web/app/components/FaceEnrollmentCapture.vue", "utf8"),
  );
  const script = compileScript(descriptor, { id: "capture-regression" });
  const filename = directory + "/capture.ts";
  writeFileSync(
    filename,
    script.content.replaceAll(
      "~/lib/biometrics",
      pathToFileURL(resolve("apps/web/app/lib/biometrics.ts")).href,
    ),
  );
  const module = await import(
    pathToFileURL(filename).href + "?" + Math.random()
  );
  const props = reactive({
    studentId: "student",
    enabled,
    busy: false,
    frames: [],
  });
  const scope = effectScope();
  const page = scope.run(() =>
    module.default.setup(props, {
      expose() {},
      emit: (_, values) => {
        if (deferEmit)
          void nextTick(() => {
            props.frames = values;
          });
        else props.frames = values;
      },
    }),
  );
  page.camera.value = { capture: () => "/9j/checked", stop() {} };
  return {
    page,
    props,
    close: () => {
      unmount();
      scope.stop();
    },
  };
}

test("capture stores the frame checked by the model, never an unchecked later frame", async () => {
  const { page, props, close } = await setup(async (_path, options) => {
    assert.equal(options.body.image, "/9j/checked");
    return { data: ready };
  });
  try {
    page.capture();
    assert.equal(props.frames.length, 0);
    page.changed(true);
    await flush();
    assert.equal(page.canCapture.value, true);
    assert.equal(page.faces.value[0].accepted, false);
    assert.equal(page.faces.value[0].tone, "success");
    page.camera.value.capture = () => "/9j/unchecked-new-person";
    page.capture();
    assert.deepEqual(props.frames, ["/9j/checked"]);
    page.capture();
    assert.equal(props.frames.length, 1);
  } finally {
    close();
  }
});

test("quality rejection, missing consent and malformed boxes cannot enable capture", async () => {
  for (const response of [
    { ...ready, quality: "low_quality" },
    { ...ready, face_count: 2 },
    { ...ready, faces: [{ box: { ...box, width: 0 } }] },
  ]) {
    const { page, props, close } = await setup(async () => ({
      data: response,
    }));
    try {
      page.changed(true);
      await flush();
      page.capture();
      assert.equal(props.frames.length, 0);
    } finally {
      close();
    }
  }
  let calls = 0;
  const { page, close } = await setup(async () => {
    calls++;
    return { data: ready };
  }, false);
  try {
    page.changed(true);
    await flush();
    assert.equal(calls, 0);
    assert.equal(page.canCapture.value, false);
  } finally {
    close();
  }
});

test("late preview results after closing are ignored and in-flight request is aborted", async () => {
  let finish, signal;
  const { page, close } = await setup(async (_path, options) => {
    signal = options.signal;
    return new Promise((resolve) => {
      finish = resolve;
    });
  });
  page.changed(true);
  close();
  assert.equal(signal.aborted, true);
  finish({ data: ready });
  await flush();
  assert.equal(page.checkedFrame.value, "");
  assert.deepEqual(page.faces.value, []);
});
test("reset after five frames resumes preview after parent props update", async () => {
  let calls = 0;
  const { page, props, close } = await setup(
    async () => {
      calls++;
      return { data: ready };
    },
    true,
    true,
  );
  try {
    props.frames = Array(5).fill("/9j/checked");
    page.changed(true);
    await flush();
    assert.equal(calls, 0);
    await page.reset();
    await flush();
    assert.equal(props.frames.length, 0);
    assert.equal(calls, 1);
    assert.equal(page.canCapture.value, true);
  } finally {
    close();
  }
});

test("changing students clears stored frames and rejects previous pending preview", async () => {
  let finish;
  const { page, props, close } = await setup(
    async () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  try {
    props.frames = ["/9j/checked"];
    page.changed(true);
    props.studentId = "other";
    await flush();
    finish({ data: ready });
    await flush();
    assert.equal(props.frames.length, 0);
    assert.equal(page.canCapture.value, false);
    assert.equal(page.active.value, false);
    assert.deepEqual(page.faces.value, []);
  } finally {
    close();
  }
});
process.on("exit", () =>
  rmSync(resolve(".tools/enrollment-regression"), {
    recursive: true,
    force: true,
  }),
);
