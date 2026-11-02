// althea-bergamot translates over stdio, for as long as its stdin stays open: Althea starts it once
// and keeps it, so that a model loads once. See docs/adr/0010-translator.md and extensions/translator.
//
// A request is a line with the pair and the size of the text in bytes, then the text:
//
//   en-pt 5\nhello
//
// The pair's model is in <pair>/config.yml under the working directory. Paths are relative on
// purpose: Marian opens files with the ANSI functions on Windows, which cannot open a path with
// an accent in it, as C:\Users\João.
//
// The answer is "ok <size>\n" and the translation, or "error <size>\n" and what went wrong. It exits
// when stdin closes.
#include <cstdio>
#include <exception>
#include <future>
#include <iostream>
#include <map>
#include <memory>
#include <sstream>
#include <string>

#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

#include "translator/parser.h"
#include "translator/response.h"
#include "translator/response_options.h"
#include "translator/service.h"

// By name, not the whole namespace: its Logger would clash with Marian's, which its macros name.
using marian::bergamot::AsyncService;
using marian::bergamot::parseOptionsFromFilePath;
using marian::bergamot::Response;
using marian::bergamot::ResponseOptions;
using marian::bergamot::TranslationModel;

namespace {

void answer(const std::string &status, const std::string &text) {
  std::cout << status << ' ' << text.size() << '\n' << text;
  std::cout.flush();
}

// isPair keeps a request to the shape of a pair, so that it cannot name a file elsewhere.
bool isPair(const std::string &pair) {
  if (pair.empty()) return false;
  for (char c : pair) {
    bool ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-';
    if (!ok) return false;
  }
  return true;
}

}  // namespace

int main() {
#ifdef _WIN32
  // The sizes count bytes: nothing may turn \n into \r\n on the way.
  _setmode(_fileno(stdin), _O_BINARY);
  _setmode(_fileno(stdout), _O_BINARY);
#endif
  std::ios::sync_with_stdio(false);

  AsyncService::Config config;
  config.numWorkers = 1;
  config.cacheSize = 0;
  AsyncService service(config);
  std::map<std::string, std::shared_ptr<TranslationModel>> models;

  std::string pair;
  size_t size = 0;
  while (std::cin >> pair >> size) {
    std::cin.ignore(1);  // the line's end
    std::string text(size, '\0');
    if (size > 0 && !std::cin.read(&text[0], static_cast<std::streamsize>(size))) break;
    try {
      if (!isPair(pair)) {
        answer("error", "not a pair of languages");
        continue;
      }
      auto &model = models[pair];
      if (!model) {
        auto options = parseOptionsFromFilePath(pair + "/config.yml");
        model = service.createCompatibleModel(options);
      }
      std::promise<Response> promise;
      auto future = promise.get_future();
      service.translate(model, std::move(text),
                        [&promise](Response &&response) { promise.set_value(std::move(response)); }, ResponseOptions{});
      answer("ok", future.get().target.text);
    } catch (const std::exception &e) {
      models.erase(pair);
      answer("error", e.what());
    }
  }
  return 0;
}
