// Copyright 2023 Jigsaw Operations LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import Foundation
import Capacitor
import SharedBackend

struct Request: Codable {
    var name: String
    var parameters: String
}

struct Response: Decodable {
    var body: String
    var error: String
}

@objc(MobileBackendPlugin)
public class MobileBackendPlugin: CAPPlugin {
    @objc func Request(_ call: CAPPluginCall) {
        let response: Response

        let encoder = JSONEncoder()
        let decoder = JSONDecoder()

        do {
            let rawRequest = try encoder.encode(
                Request(
                    name: call.getString("name")!,
                    parameters: call.getString("parameters")!
                )
            )
            
            response = try decoder.decode(
                Response.self,
                from: Shared_backendHandleRequest(rawRequest)!
            )
        } catch {
            return call.resolve([
                "error": error
            ])
        }
            
        return call.resolve([
            "body": response.body,
            "error": response.error
        ])
    }
}